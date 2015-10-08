package main

import (
	"fmt"

	"github.com/pavlik/mimozaflowers.ru/Godeps/_workspace/src/github.com/labstack/echo"
	mw "github.com/pavlik/mimozaflowers.ru/Godeps/_workspace/src/github.com/labstack/echo/middleware"
	"github.com/pavlik/mimozaflowers.ru/Godeps/_workspace/src/golang.org/x/net/websocket"
)

func main() {
	e := echo.New()

	e.Use(mw.Logger())
	e.Use(mw.Recover())

	e.Static("/", "public")
	e.WebSocket("/ws", func(c *echo.Context) (err error) {
		ws := c.Socket()
		msg := ""

		for {
			if err = websocket.Message.Send(ws, "Hello, Client!"); err != nil {
				return
			}
			if err = websocket.Message.Receive(ws, &msg); err != nil {
				return
			}
			fmt.Println(msg)
		}
		return
	})

	e.Run(":1323")
}
