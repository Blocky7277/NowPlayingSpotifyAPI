package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"github.com/joho/godotenv"
)


var (
	clientID, clientSecret, refreshToken string

	mu      sync.Mutex
	token   string
	expires time.Time
)

type spotifyMetaData struct {
	IsPlaying  bool   `json:"isPlaying"`
	Title      string `json:"title,omitempty"`
	Artist     string `json:"artist,omitempty"`
	Art        string `json:"art,omitempty"`
	ProgressMs int    `json:"progressMs,omitempty"`
	DurationMs int    `json:"durationMs,omitempty"`

}

func accessToken() (string, error) {
	mu.Lock();
	defer mu.Unlock();
	if token != "" && time.Now().Before(expires) {
		return token, nil
	}
	form := url.Values{
		"grant_type": {"refresh_token"}, 
		"refresh_token": {refreshToken},
	}
	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(form.Encode()))
	if (err != nil) {
		log.Println("couldn't create new req")
		return "", err
	}
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println("req failed")
		return "", err
	}

	defer resp.Body.Close()
	var d struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil || d.AccessToken == "" {
		log.Println(resp.StatusCode)
		return "", errors.New("token refresh failed")
	}
	token = d.AccessToken
	expires = time.Now().Add(time.Duration(d.ExpiresIn-60) * time.Second)
	log.Println("token received")
	return token, nil
}

func handleNowPlaying(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// w.Header().Set("Access-Control-Allow-Origin", "")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	var out spotifyMetaData	
	defer func() { json.NewEncoder(w).Encode(out) }() // empty = notplaying

	tok, err := accessToken()
	if err != nil {
		log.Println("Token access denied")
		return
	}

	req, _ := http.NewRequest("GET", "https://api.spotify.com/v1/me/player/currently-playing", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK { // 204 = nothing playing
		log.Println(resp.StatusCode)
		return
	}

	var d struct {
		IsPlaying  bool `json:"is_playing"`
		ProgressMs int  `json:"progress_ms"`
		Item       *struct {
			Name       string `json:"name"`
			DurationMs int    `json:"duration_ms"`
			Artists    []struct {
				Name string `json:"name"`
			} `json:"artists"`
			Album struct {
				Images []struct {
					URL string `json:"url"`
				} `json:"images"`
			} `json:"album"`
		} `json:"item"`
	}

	if json.NewDecoder(resp.Body).Decode(&d) != nil || d.Item == nil {
		return
	}

	names := make([]string, 0, len(d.Item.Artists))
	for _, a := range d.Item.Artists {
		names = append(names, a.Name)
	}
	out = spotifyMetaData {
		IsPlaying:  d.IsPlaying,
		Title:      d.Item.Name,
		Artist:     strings.Join(names, ", "),
		ProgressMs: d.ProgressMs,
		DurationMs: d.Item.DurationMs,
	}
	if imgs := d.Item.Album.Images; len(imgs) > 1 {
		out.Art = imgs[1].URL
	} else if len(imgs) == 1 {
		out.Art = imgs[0].URL
	}
}

func main() {
	_ = godotenv.Load()
	clientID = os.Getenv("CLIENT_ID")
	clientSecret = os.Getenv("CLIENT_SECRET")
	refreshToken = os.Getenv("REFRESH_TOKEN")
	log.Println(refreshToken)
	http.HandleFunc("/api/now-playing", handleNowPlaying)
	log.Println("listening on :3000")
	log.Fatal(http.ListenAndServe(":3000", nil))
}
