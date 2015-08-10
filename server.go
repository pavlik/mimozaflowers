package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo"
	mw "github.com/labstack/echo/middleware"
	"github.com/spf13/viper"
)

type config struct {
	ClientID     string
	ClientSecret string
	BaseURL      string
	Username     string
}

var C config

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
	response, err := http.Get(C.BaseURL + "/users/search?q=" + username + "&client_id=" + C.ClientID)
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

	response, err := http.Get(C.BaseURL + "/users/" + userID + "/media/recent/?client_id=" + C.ClientID + "&count=" + strconv.Itoa(count))
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
	id, err := getUserID("mimozaflowers")
	if err != nil {
		return c.String(http.StatusOK, err.Error())
	}

	mediasResponse, err2 := getRecentMedia(id, 20)

	if err2 != nil {
		return c.String(http.StatusOK, err.Error())
	}

	if mediasResponse.Meta.Code == 200 {
		return c.Render(http.StatusOK, "index", mediasResponse)
	}

	return c.JSON(http.StatusOK, mediasResponse)
}

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func main() {
	// init config
	viper.SetConfigName("config") // name of config file (without extension)

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	err = viper.Marshal(&C)
	if err != nil {
		log.Fatalf("unable to decode into struct, %v", err)
	}

	// Echo instance
	e := echo.New()
	e.SetDebug(true)

	t := &Template{templates: template.Must(template.ParseGlob("templates/*.html"))}
	e.SetRenderer(t)

	// Middleware
	e.Use(mw.Logger())
	e.Use(mw.Recover())

	// Routes
	e.Static("/js/", "public/js")
	e.Static("/css/", "public/css")
	e.Get("/", recentMedia)

	// Start server
	e.Run(":3000")
}