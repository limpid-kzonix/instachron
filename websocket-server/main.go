package main

import (
	"crypto/rand"
	"io"
	"log"
	"net/http"
	"time"

	"os"

	"math/big"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}

const (
	letterBytes   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

func generateRandomString(n int) string {
	b := make([]byte, n)
	for i, cache, remain := n-1, new(big.Int).SetInt64(0), 0; i >= 0; {
		if remain == 0 {
			cache, _ = rand.Int(rand.Reader, big.NewInt(int64(1<<62)))
			remain = letterIdxMax
		}
		if idx := int(cache.Int64() & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache.Rsh(cache, letterIdxBits)
		remain--
	}
	return string(b)
}

func ws(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("recv: %s", message)
		megabyte := 1024 * 1024
		randomString := generateRandomString(megabyte)
		str := "Hello, World! -- " + string(message) + " :::" + randomString
		binaryData := []byte(str)
		err = c.WriteMessage(websocket.BinaryMessage, binaryData)
		if err != nil {
			log.Println("write:", err)
			break
		}
	}
}

func writeByteArrayToFile(data []byte, filename string) error {
	// Create or truncate the file
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write the byte array to the file
	_, err = file.Write(data)
	if err != nil {
		return err
	}

	return nil
}

func upload(resp http.ResponseWriter, req *http.Request) {

	start := time.Now()

	log.Println("Handling request")
	buffer, err := io.ReadAll(req.Body)
	if err != nil {
		log.Println("Fail: ", err)
	}
	err = writeByteArrayToFile(buffer, "./stage.jpeg")

	if err != nil {
		log.Println("Fail: ", err)
		io.WriteString(resp, "Image not uploaded")
	} else {
		io.WriteString(resp, "Image uploaded")
	}

	os.Rename("./stage.jpeg", "./current-image.jpeg")

	diff := time.Now().Sub(start)

	log.Println(diff)

	return
}

func main() {
	log.Println("Starting application")
	http.HandleFunc("/stream", ws)
	http.HandleFunc("/upload", upload)

	err := http.ListenAndServe("0.0.0.0:5000", nil)

	if err == nil {
		log.Println("Server started.")
	} else {
		log.Println("Failed: ", err)
	}
}
