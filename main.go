package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	uuid "github.com/satori/go.uuid"
	"net/http"
)

type Client struct {
	id     string
	socket *websocket.Conn
	send   chan []byte
}
type ClientManager struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	unregister chan *Client
	register   chan *Client
}

type Message struct {
	Sender    string `json:"sender,omitempty"` //省略空的 omit empty
	Recipient string `json:"recipient,omitempty"`
	Content   string `json:"content,omitempty"`
}

func (manager *ClientManager) start() { //ref 引用
	for {
		//loop
		//多路复用
		select {
		//感知注册客户端的情况
		case conn := <-manager.register:
			//已注册状态设置为true
			manager.clients[conn] = true
			//json 序列化Message对象 &指向对象内容
			jsonMsg, _ := json.Marshal(&Message{Content: "/A new socket has connected."})
			manager.send(jsonMsg, conn)

			//离线
		case conn := <-manager.unregister:
			//客户端是否已连接
			if _, ok := manager.clients[conn]; ok {
				//关闭正在发送的操作
				close(conn.send)
				//删除这个连接
				delete(manager.clients, conn)
				jsonMsg, _ := json.Marshal(&Message{Content: "/A socket has disconnected"})
				manager.send(jsonMsg, conn)
			}

			//广播消息
		case msg := <-manager.broadcast:
			//for range 遍历
			for conn := range manager.clients {
				select {
				//往每个连接里传送msg
				case conn.send <- msg:
				default:
					close(conn.send)
					delete(manager.clients, conn)
				}
			}
		}
	}

}

/*
*
接收到消息后，排除自己全部发送
*/
func (manager *ClientManager) send(msg []byte, ignore *Client) {
	for conn := range manager.clients {
		if conn != ignore {
			conn.send <- msg
		}
	}
}

var manager *ClientManager

/*
*方法接收器
一个值或一个指针类型
读
*/
func (c *Client) read() {
	defer func() {
		manager.unregister <- c
		c.socket.Close()
	}()

	for {
		_, msg, err := c.socket.ReadMessage()
		if err != nil {
			manager.unregister <- c
			c.socket.Close()
			break
		}
		jsonMsg, _ := json.Marshal(&Message{Sender: c.id, Content: string(msg)})
		manager.broadcast <- jsonMsg
	}
}

func (c *Client) write() {
	defer func() {
		c.socket.Close()
	}()

	for {
		select {
		//获取写入通道数据
		case msg, ok := <-c.send:
			if !ok {
				c.socket.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.socket.WriteMessage(websocket.TextMessage, msg)
		}
	}
}

func wsPage(resWriter http.ResponseWriter, req *http.Request) {
	//一行过长换行
	//http 请求升级为websocket请求 upgrade to websocket request
	conn, err := (&websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}).
		Upgrade(resWriter, req, nil)
	if err != nil {
		http.NotFound(resWriter, req)
		return
	}
	client := &Client{id: uuid.NewV4().String(), socket: conn, send: make(chan []byte)}

	manager.register <- client

	go client.read()
	go client.write()
}

func main() {
	fmt.Println("Starting application")
	go manager.start()
	http.HandleFunc("/ws", wsPage)
	http.ListenAndServe(":12345", nil)
}
