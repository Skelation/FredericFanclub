package hub

import (
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

type PropMarket struct {
	Player    string   `json:"player"`
	PropType  string   `json:"prop_type"`
	Line      float64  `json:"line"`
	OverMult  float64  `json:"over_multiplier"`
	UnderMult float64  `json:"under_multiplier"`
	IsOpen    bool     `json:"is_open"`
	Vetoes    []string `json:"vetoes"`
}

type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

var (
	DevMode = devModeFromEnv()

	CurrentMarket *PropMarket
	PuuidCache    sync.Map

	Upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	Clients   = make(map[*websocket.Conn]bool)
	Mu        sync.Mutex
	Broadcast = make(chan WSMessage)
)

func devModeFromEnv() bool {
	return strings.EqualFold(os.Getenv("FRED_ENV"), "dev") ||
		strings.EqualFold(os.Getenv("FRED_ENV"), "development")
}

// RefreshDevMode re-reads FRED_ENV. The package-level DevMode initializer runs
// before main() loads the .env file, so call this after LoadDotEnv() for the
// flag to pick up values that live only in .env.
func RefreshDevMode() { DevMode = devModeFromEnv() }

func StartBroadcaster() {
	go func() {
		for {
			msg := <-Broadcast
			Mu.Lock()
			for client := range Clients {
				if err := client.WriteJSON(msg); err != nil {
					client.Close()
					delete(Clients, client)
				}
			}
			Mu.Unlock()
		}
	}()
}
