package main

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
)

//Hashmap of registered users and known connections - in ram
var USERS map[string]User
var CONNECTIONS map[*websocket.Conn]string

// Template of a User
type User struct {
	Name string
	Peer string
	Conn *websocket.Conn
}

//Define upgrade policy.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkOrigin,
}

// Template of input message readable by the server
type SignalMessage struct {
	Type      string     `json:"type,omitempty"`
	Name      string     `json:"name,omitempty"`
	Offer     *Offer     `json:"offer,omitempty"`
	Answer    *Answer    `json:"answer,omitempty"`
	Candidate *Candidate `json:"candidate,omitempty"`
}

// Define Login request sent back from the server.
// External struct to manage Success bool independently
type LoginResponse struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
}

type Offer struct {
	Type string `json:"type"`
	Sdp  string `json:"sdp"`
}

type Answer struct {
	Type string `json:"type"`
	Sdp  string `json:"sdp"`
}

type Candidate struct {
	Candidate     string `json:"candidate"`
	SdpMid        string `json:"sdpMid"`
	SdpMLineIndex int    `json:"sdpMLineIndex"`
}

type Leaving struct {
	Type string `json:"type"`
}

type DefaultError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

//Ugrade policty from http request to websocket, to be defined
func checkOrigin(r *http.Request) bool {
	//For example: Check in a blacklist if the address is present
	//if blacklist_check(r.RemoteAddr) { return false }
	return true
}

//Forward Offer to remote peer
func onOffer(data SignalMessage, conn *websocket.Conn) (err error) {
	log.Println("Offer received from", CONNECTIONS[conn])
	var sm SignalMessage

	sm.Offer = data.Offer
	sm.Type = "offer"

	if peer, isRegistered := USERS[data.Name]; isRegistered {
		sm.Name = CONNECTIONS[conn]
		out, err := json.Marshal(sm)
		if err != nil {
			log.Println("Error - onOffer - Marshal:", err)
			return err
		}
		if err = peer.Conn.WriteMessage(1, out); err != nil {
			log.Println("Error - onOffer - WriteMessage:", err)
			return err
		}
		log.Println("Offer forwarded to", peer.Name)
	} else {
		log.Println("Error - Can not send offer to an unregistered peer.")
		return err
	}
	return nil
}

//Forward Answer to original peer
func onAnswer(data SignalMessage, conn *websocket.Conn) (err error) {
	log.Println("Answer received from", CONNECTIONS[conn])
	var sm SignalMessage

	sm.Answer = data.Answer
	sm.Type = "answer"

	if peer, isRegistered := USERS[data.Name]; isRegistered {
		out, err := json.Marshal(sm)
		if err != nil {
			log.Println("Error - onAnswer - Marshal:", err)
			return err
		}
		if err = peer.Conn.WriteMessage(1, out); err != nil {
			log.Println("Error - onAnswer - WriteMessage:", err)
			return err
		}
		log.Println("Answer forwarded to", peer.Name)
	} else {
		log.Println("Error - Can not send answer to an unregistered peer")
		return err
	}
	return nil
}

//Forward candidate to original peer
func onCandidate(data SignalMessage, conn *websocket.Conn) (err error) {
	log.Println("Candidate received from", CONNECTIONS[conn])
	var sm SignalMessage

	sm.Candidate = data.Candidate
	sm.Type = "candidate"

	if peer, isRegistered := USERS[data.Name]; isRegistered {
		out, err := json.Marshal(sm)
		if err != nil {
			log.Println("Error - onCandidate - Marshal:", err)
			return err
		}
		if err = peer.Conn.WriteMessage(1, out); err != nil {
			log.Println("Error - onCandidate - WriteMessage:", err)
			return err
		}
		log.Println("Candidate forwarded to", peer.Name)
	} else {
		log.Println("Error - Can not send candidate to an unregistered peer")
		return err
	}
	return nil
}

//Forward leave message to remote Peer and close the current connection
func onLeave(data SignalMessage, conn *websocket.Conn) (err error) {
	var out []byte
	defer conn.Close()

	log.Println("Leave message received from", CONNECTIONS[conn])

	out, err = json.Marshal(Leaving{Type: "leaving"})
	if err != nil {
		log.Println("Error - onLeaving - Marshal:", err)
		return err
	}

	user := CONNECTIONS[conn]
	if peer, isRegistered := USERS[USERS[user].Peer]; isRegistered {
		if err = peer.Conn.WriteMessage(1, out); err != nil {
			log.Println("Error - onLeaving - WriteMessage:", err)
			return err
		}
		log.Println("Leaving message sent to", peer.Name)
	} else {
		log.Println("Error - Can not send leaving message to remote peer.")
		return err
	}
	return nil
}

func onLogin(data SignalMessage, conn *websocket.Conn) (err error) {
	var out []byte

	if _, isRegistered := USERS[data.Name]; isRegistered {
		out, err = json.Marshal(LoginResponse{Type: "login", Success: false})
		log.Println("User", CONNECTIONS[conn], "tried but was not allowed to log in")
	} else {
		USERS[data.Name] = User{Name: data.Name, Conn: conn}
		CONNECTIONS[conn] = data.Name
		out, err = json.Marshal(LoginResponse{Type: "login", Success: true})
		log.Println("User", CONNECTIONS[conn], "logged in successfully")
		if err != nil {
			log.Println("Error - onLogin - Marshal:", err)
			return err
		}
		if err = conn.WriteMessage(1, out); err != nil {
			log.Println("Error - onLogin - WriteMessage:", err)
			return err
		}
	}
	return nil
}

func onDefault(raw []byte, conn *websocket.Conn) (err error) {
	var out []byte
	var message SignalMessage

	err = json.Unmarshal(raw, &message)
	if err != nil {
		log.Println("Error - onDefault - Unmarshal:", err)
		return
	}

	if message.Type != "" {
		log.Println("Unrecognized type command found:", message.Type)
	} else {
		log.Println("Unrecognized type command found:", string(raw))
	}

	out, err = json.Marshal(DefaultError{Type: "error", Message: "Unrecognized command"})
	if err != nil {
		log.Println("Error - onDefault - Marhshal:", err)
		return err
	}
	if err = conn.WriteMessage(1, out); err != nil {
		log.Println("Error - onDefault - WriteMessage:", err)
		return err
	}
	return nil
}

func connHandler(conn *websocket.Conn) {
	_, raw, err := conn.ReadMessage()
	var message SignalMessage

	if err != nil {
		log.Println("Error - connHandler - ReadMessage:", err)
		return
	}
	err = json.Unmarshal(raw, &message)

	if err != nil {
		log.Println("Error - connHandler - Unmarshal - Incorrect data format:", string(raw), ":", err)
		out, err := json.Marshal(DefaultError{Type: "error", Message: "Incorrect data format"})
		if err != nil {
			log.Println("Error - connHandler - MarshalError:", err)
			return
		}
		if err = conn.WriteMessage(1, out); err != nil {
			log.Println("Error - connHandler- WriteMessage Response:", err)
		}
		return
	}
	messageInputType := message.Type

	switch messageInputType {
	case "login":
		onLogin(message, conn)
	case "offer":
		onOffer(message, conn)
	case "answer":
		onAnswer(message, conn)
	case "candidate":
		onCandidate(message, conn)
	case "leave":
		onLeave(message, conn)
	default:
		onDefault(raw, conn)
	}
}

//Catches HTTP Requests, upgrade them if needed and let connHandler managing the connection
func reqHandler(w http.ResponseWriter, r *http.Request) {
	//Upgrade a HTTP Request to get a pointer to a Conn
	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	for {
		connHandler(conn)
	}
}

func main() {
	USERS = make(map[string]User)
	CONNECTIONS = make(map[*websocket.Conn]string)
	http.HandleFunc("/", reqHandler)
	log.Println("Signaling Server started")
	err := http.ListenAndServe(":9090", nil)
	if err != nil {
		log.Println("Error: " + err.Error())
	}
}
