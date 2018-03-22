package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/keith-turner/ecoji"

	"github.com/go-chi/chi"

	bolt "github.com/coreos/bbolt"
)

type URLStruct struct {
	URL string `json:"url"`
}

var host = "https://urlji.xyz"
var dataStore *bolt.DB
var bucket = []byte("IDs")

func main() {
	var err error
	dataStore, err = bolt.Open("urls.db", 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		log.Fatal(err)
	}

	err = dataStore.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucket)
		return err
	})
	if err != nil {
		log.Fatal(err)
	}
	defer dataStore.Close()

	router := chi.NewRouter()
	router.Get("/", fileServer)
	router.Get("/{id}", getURL)
	router.Post("/url", createURL)

	http.ListenAndServe(":8080", router)
}

func getURLKey() ([]byte, error) {
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, err
	}
	emojiBytes := bytes.NewBuffer(nil)
	err := ecoji.Encode(bytes.NewReader(randomBytes), emojiBytes, 0)

	return emojiBytes.Bytes(), err
}

func fileServer(w http.ResponseWriter, r *http.Request) {
	http.FileServer(http.Dir("static")).ServeHTTP(w, r)
}

func createURL(w http.ResponseWriter, r *http.Request) {
	newURL := URLStruct{}
	err := json.NewDecoder(r.Body).Decode(&newURL)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	defer r.Body.Close()

	var URLKey []byte

	err = dataStore.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucket)

		if bucket == nil {
			return errors.New("No bucket")
		}

		// first check to see if this url is already in the database
		cursor := bucket.Cursor()
		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			if newURL.URL == string(value) {
				URLKey = key
				return nil
			}
		}

		var urlErr error
		for {
			URLKey, urlErr = getURLKey()
			if urlErr != nil {
				return urlErr
			}
			if exists := bucket.Get(URLKey); exists == nil {
				break
			}
		}

		if putErr := bucket.Put(URLKey, []byte(newURL.URL)); putErr != nil {
			return putErr
		}
		return nil
	})

	if err != nil {
		w.WriteHeader(500)
		return
	}

	shortURL := URLStruct{
		URL: fmt.Sprintf("%s/%s", host, URLKey),
	}
	returnJSON, err := json.Marshal(shortURL)
	if err != nil {
		w.WriteHeader(500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(returnJSON)
}

func getURL(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var url []byte
	dataStore.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucket)
		url = bucket.Get([]byte(id))
		return nil
	})

	if url != nil {
		http.Redirect(w, r, string(url), http.StatusFound)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}
