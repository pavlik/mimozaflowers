package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"time"

	"log"

	"github.com/labstack/echo"
	mw "github.com/labstack/echo/middleware"
	"github.com/patrickmn/go-cache"
	"github.com/spf13/viper"
)

type config struct {
	CLIENTID     string
	CLIENTSECRET string
	BASEURL      string
	USERNAME     string
	PORT         string
}

var C config

var InstaCache *cache.Cache

type UserResponse struct {
	MetaResponse
	User *User `json:"data"`
}

type UsersResponse struct {
	MetaResponse
	Users []User `json:"data"`
}

type MediaResponse struct {
	MetaResponse
	Media *Media `json:"data"`
}

type MediasResponse struct {
	MetaResponse
	Medias []Media `json:"data"`
}

type MetaResponse struct {
	Meta *Meta
}

type User struct {
	Username       string `json:"username"`
	FirstName      string `json:"first_name"`
	ProfilePicture string `json:"profile_picture"`
	ID             string `json:"id"`
	LastName       string `json:"last_name"`
}

type Meta struct {
	Code         int
	ErrorType    string `json:"error_type"`
	ErrorMessage string `json:"error_message"`
}

// Instagram Media object
type Media struct {
	Type         string
	Id           string
	UsersInPhoto []UserPosition `json:"users_in_photo"`
	Filter       string
	Tags         []string
	Comments     *Comments
	Caption      *Caption
	Likes        *Likes
	Link         string
	User         *User
	CreatedTime  StringUnixTime `json:"created_time"`
	Images       *Images
	Videos       *Videos
	Location     *Location
	UserHasLiked bool `json:"user_has_liked"`
	Attribution  *Attribution
}

// A pair of user object and position
type UserPosition struct {
	User     *User
	Position *Position
}

// A position in a media
type Position struct {
	X float64
	Y float64
}

// Instagram tag
type Tag struct {
	MediaCount int64 `json:"media_count"`
	Name       string
}

type Comments struct {
	Count int64
	Data  []Comment
}

type Comment struct {
	CreatedTime StringUnixTime `json:"created_time"`
	Text        string
	From        *User
	Id          string
}

type Caption Comment

type Likes struct {
	Count int64
	Data  []User
}

type Images struct {
	LowResolution      *Image `json:"low_resolution"`
	Thumbnail          *Image
	StandardResolution *Image `json:"standard_resolution"`
}

type Image struct {
	Url    string
	Width  int64
	Height int64
}

type Videos struct {
	LowResolution      *Video `json:"low_resolution"`
	StandardResolution *Video `json:"standard_resolution"`
}

type Video Image

type Location struct {
	Id        LocationId
	Name      string
	Latitude  float64
	Longitude float64
}

type Relationship struct {
	IncomingStatus string `json:"incoming_status"`
	OutgoingStatus string `json:"outgoing_status"`
}

// If another app uploaded the media, then this is the place it is given. As of 11/2013, Hipstamic is the only allowed app
type Attribution struct {
	Website   string
	ItunesUrl string
	Name      string
}

type StringUnixTime string

func (s StringUnixTime) Time() (t time.Time, err error) {
	unix, err := strconv.ParseInt(string(s), 10, 64)
	if err != nil {
		return
	}

	t = time.Unix(unix, 0)
	return
}

// Sometimes location Id is a string and sometimes its an integer
type LocationId interface{}

func ParseLocationId(lid LocationId) string {
	if lid == nil {
		return ""
	}
	if slid, ok := lid.(string); ok {
		return slid
	}
	if ilid, ok := lid.(int64); ok {
		return fmt.Sprintf("%d", ilid)
	}
	return ""
}

func getUserID(username string) (string, error) {
	usersResponse := new(UsersResponse)
	response, err := http.Get(C.BASEURL + "/users/search?q=" + username + "&client_id=" + C.CLIENTID)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if err = json.NewDecoder(response.Body).Decode(&usersResponse); err != nil {
		return "", err
	}

	if usersResponse.Meta.Code == 200 {
		// search for id of given user in array
		for _, value := range usersResponse.Users {
			if value.Username == username {
				return value.ID, nil
			}
		}
	}

	return "", errors.New(usersResponse.Meta.ErrorMessage)
}

func getRecentMedia(userID string, count int) (*MediasResponse, error) {
	mediasResponse := new(MediasResponse)

	response, err := http.Get(C.BASEURL + "/users/" + userID + "/media/recent/?client_id=" + C.CLIENTID + "&count=" + strconv.Itoa(count))
	if err != nil {
		return mediasResponse, err
	}
	defer response.Body.Close()
	if err = json.NewDecoder(response.Body).Decode(&mediasResponse); err != nil {
		return mediasResponse, err
	}

	if mediasResponse.Meta.Code == 200 {
		return mediasResponse, nil
	}

	return mediasResponse, errors.New(mediasResponse.Meta.ErrorMessage)
}

// Handler
func recentMedia(c *echo.Context) error {

	var err error

	//log.Println("InstaChache: ", InstaCache)

	// Caching UserID
	var id string
	x, found := InstaCache.Get("userID")
	if found {
		id = x.(string)
	} else {
		id, err = getUserID(C.USERNAME)
		if err != nil {
			log.Println("Error in getUserID func: ", err)
			err = c.String(http.StatusBadRequest, "User not found: "+err.Error())
		}
		if err == nil {
			InstaCache.Set("userID", id, cache.DefaultExpiration)
		}
	}

	//log.Println("UserID: ", id)

	// Caching Instafeed last 20 photos
	var mediasResponse *MediasResponse
	if x, found := InstaCache.Get("mediasResponse"); found {
		mediasResponse = x.(*MediasResponse)
	} else {
		mediasResponse, err = getRecentMedia(id, 20)

		if err != nil {
			log.Println("Error in getRecentMedia: ", err)
			err = c.String(http.StatusBadRequest, "Media not found: "+err.Error())
		}

		if err == nil {
			InstaCache.Set("mediasResponse", mediasResponse, cache.DefaultExpiration)
		}
	}

	if mediasResponse.Meta.Code == 200 {
		err = c.Render(http.StatusOK, "index", mediasResponse)
	}

	return err
}

func generateEndingColumns(columnsToGenerate, columns int) string {
	var feed string
	for i := 1; i <= columnsToGenerate; i++ {
		feed += `<div class="col-sm-6 col-md-` + strconv.Itoa(columns) + `"></div>`
		if i == columnsToGenerate {
			feed += `</div>`
		}
	}

	return feed
}

func buildInstaFeed(medias []Media, itemsPerRow int) template.HTML {
	counter := 1
	var feed string
	columns := 12 / itemsPerRow
	itemsCount := len(medias)
	rows := itemsCount / itemsPerRow
	rowsMod := itemsCount % itemsPerRow
	if rowsMod > 0 {
		rows++
	}
	realItemsCount := itemsPerRow * rows
	endingEmptyColumns := realItemsCount - itemsCount

	for i := 0; i < itemsCount; i++ {
		imageURL := medias[i].Images.LowResolution.Url
		link := medias[i].Link

		if counter == 1 {
			feed += `<div class="insta row">`
			feed += `<div class="col-sm-6 col-md-` + strconv.Itoa(columns) + `">`
			feed += `<a href="` + link + `"><img src="` + imageURL + `" alt="" class="img-responsive"></a>`
			feed += `</div>`
			counter++
		} else if counter == itemsPerRow {
			feed += `<div class="col-sm-6 col-md-` + strconv.Itoa(columns) + `">`
			feed += `<a href="` + link + `"><img src="` + imageURL + `" alt="" class="img-responsive"></a>`
			feed += `</div>`
			feed += `</div>`
			counter = 1
		} else {
			feed += `<div class="col-sm-6 col-md-` + strconv.Itoa(columns) + `">`
			feed += `<a href="` + link + `"><img src="` + imageURL + `" alt="" class="img-responsive"></a>`
			feed += `</div>`
			counter++
		}

		feed += generateEndingColumns(endingEmptyColumns, columns)
	}

	return template.HTML(feed)
}

type Template struct {
	templates *template.Template
}

// Render HTML
func (t *Template) Render(w io.Writer, name string, data interface{}) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func main() {
	// init config
	viper.SetConfigName("config") // name of config file (without extension)
	viper.AddConfigPath(".")

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {
		log.Println(err)
		// Handle errors reading the config file
		viper.AutomaticEnv()
		//panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	// err = viper.Marshal(&C)
	// if err != nil {
	// 	log.Fatalf("unable to decode into struct, %v", err)
	// }
	//os.Getenv("PORT")

	// log.Println("Mimozaflowers port: ", viper.GetString("port"))

	C = config{CLIENTID: viper.GetString("clientid"),
		CLIENTSECRET: viper.GetString("clientsecret"),
		BASEURL:      viper.GetString("baseurl"),
		USERNAME:     viper.GetString("username"),
		PORT:         viper.GetString("port")}

	// set cache for data from instagram
	InstaCache = cache.New(5*time.Hour, 30*time.Minute)

	// Gin instance
	router := echo.New() //gin.Default()

	html := template.Must(template.New("").Funcs(template.FuncMap{"buildInstaFeed": buildInstaFeed}).ParseGlob("templates/*.html"))
	t := &Template{templates: html}
	router.SetRenderer(t)
	//router.SetHTMLTemplate(html)

	// Middleware
	router.Use(mw.Logger())
	router.Use(mw.Recover())

	// Routes
	router.Static("/js/", "public/js")
	router.Static("/css/", "public/css")
	router.Get("/", recentMedia)

	// Start server
	//err = router.Run(":" + C.PORT)

	//graceful.Run(":"+C.PORT, 5*time.Second, router)
	//graceful.ListenAndServe(router.Server(":"+C.PORT), 5*time.Second)
	router.Run(":" + C.PORT)
	// if err != nil {
	// 	log.Println("Error: ", err)
	// }

}
