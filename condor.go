package main

import (
	"flag"
	"fmt"
	"time"

	irc "github.com/fluffle/goirc/client"
	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/websocket"
	"github.com/satori/go.uuid"
	"github.com/tidwall/gjson"
)

func followAll(c *websocket.Conn) {
	err := c.WriteMessage(websocket.TextMessage, []byte(`{"id":"`+uuid.NewV4().String()+`","method":"stream.follow","params":{"type":"location","count":1,"order":"newest","follow":true}}`))

	if err != nil {
		panic(err)
	}
}

var (
	muted = make(map[string]bool)
)

func fenceListen(b *irc.Conn, server *string) {
	r, err := redis.Dial("tcp", *server)
	if err != nil {
		panic(err)
	}
	defer r.Close()

	r.Do("nearby", "friends", "fence", "roam", "friends", "*", "1000")
	for {
		resp, err := r.Receive()
		if err != nil {
			fmt.Println(err)
			panic(err)
		}
		j := string(resp.([]byte))
		fmt.Println(j)
		d := gjson.Get(j, "detect").String()
		f1 := gjson.Get(j, "id").String()
		f2 := gjson.Get(j, "nearby.id").String()
		go func() {
			time.Sleep(time.Minute * 5)
			muted[f1] = false
		}()
		if d != "" && !muted[f1] {
			b.Privmsg("#pdxbots", f1+" is now within a km of "+f2)
		}
		muted[f1] = true
	}

}

func main() {
	server := flag.String("server", "localhost:9851", "redis server location")
	token := flag.String("token", "", "icecondor token")
	nick := flag.String("nick", "locbot", "nick of irc bot")

	flag.Parse()
	b := irc.SimpleClient(*nick)
	b.HandleFunc(irc.CONNECTED,
		func(conn *irc.Conn, line *irc.Line) { conn.Join("#pdxbots") })
	if err := b.ConnectTo("irc.freenode.net"); err != nil {
		panic(err)
	}
	r, err := redis.Dial("tcp", *server)
	if err != nil {
		panic(err)
	}
	defer r.Close()
	friends := make(map[string]string)
	c, _, err := websocket.DefaultDialer.Dial("wss://api.icecondor.com/v2", nil)
	if err != nil {
		panic(err)
	}

	authID := uuid.NewV4().String()
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			panic(err)
		}
		msg := string(message)
		fmt.Println(msg)
		stream := gjson.Get(msg, "result.added.0.id").String()
		userID := gjson.Get(msg, "result.added.0.username").String()
		lat := gjson.Get(msg, "result.latitude").String()
		lon := gjson.Get(msg, "result.longitude").String()
		ID := gjson.Get(msg, "result.user_id").String()
		date := gjson.Get(msg, "result.date").String()
		method := gjson.Get(msg, "method").String()
		reqID := gjson.Get(msg, "id").String()

		if method == "hello" {
			req := `{"id":"` + authID + `","method":"auth.session","params":{"device_key":"` + *token + `"}}`
			err = c.WriteMessage(websocket.TextMessage, []byte(req))
			if err != nil {
				panic(err)
			}
		}
		if reqID == authID {
			followAll(c)
			go fenceListen(b, server)
		}
		if stream != "" {
			friends[stream] = userID
		}
		if lat != "" {
			if err != nil {
				panic(err)
			}
			p, err := time.Parse(time.RFC3339Nano, date)
			if err != nil {
				panic(err)
			}

			_, err = r.Do("set", "friends", friends[ID], "point", lat, lon, p.Unix())
			if err != nil {
				panic(err)
			}
		}
	}
}
