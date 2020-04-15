// Package weather implements a Service which adds !commands for openweather search
package weather

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/davecgh/go-spew/spew"
	"github.com/matrix-org/go-neb/types"
	"github.com/matrix-org/gomatrix"
)

// ServiceType of the Weather service
const ServiceType = "weather"

const apiBase = "https://api.openweathermap.org/data/2.5/weather"

var httpClient = &http.Client{}

type (
	coord struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	}

	weather struct {
		ID          int    `json:"id"`
		Main        string `json:"main"`
		Description string `json:"description"`
		Icon        string `json:"icon"`
	}

	mainInfo struct {
		Temp        float64 `json:"temp"`
		FeelsLike   float64 `json:"feels_like"`
		TempMin     float64 `json:"temp_min"`
		TempMax     float64 `json:"temp_max"`
		Pressure    float64 `json:"pressure"`
		Humidity    float64 `json:"humidity"`
		SeaLevel    float64 `json:"sea_level"`
		GroundLevel float64 `json:"grnd_level"`
	}

	wind struct {
		Speed float64 `json:"speed"`
		Deg   float64 `json:"deg"`
	}

	clouds struct {
		All float64 `json:"all"`
	}

	rain struct {
		Hour1 float64 `json:"1h"`
		Hour3 float64 `json:"3h"`
	}

	snow struct {
		Hour1 float64 `json:"1h"`
		Hour3 float64 `json:"3h"`
	}

	sys struct {
		Country string `json:"country"`
		Sunrise int64  `json:"sunrise"`
		Sunset  int64  `json:"sunset"`
	}

	weatherResponse struct {
		ID         int64     `json:"id"`
		Name       string    `json:"name"`
		Coord      coord     `json:"coord"`
		Weather    []weather `json:"weather"`
		Base       string    `json:"base"`
		Main       mainInfo  `json:"main"`
		Visibility float64   `json:"visibility"`
		Wind       wind      `json:"wind"`
		Rain       rain      `json:"rain"`
		Snow       snow      `json:"snow"`
		Dt         int64     `json:"dt"`
		Sys        sys       `json:"sys"`
		Timezone   int       `json:"timezone"`
	}
)

// Service contains the Config fields for the Weather service.
//
// Example request:
//   {
//			"api_key": "AIzaSyA4FD39..."
//   }
type Service struct {
	types.DefaultService
	APIKey string `json:"api_key"`
}

// Commands supported:
//    !imgur some_search_query_without_quotes
// Responds with a suitable image into the same room as the command.
func (s *Service) Commands(client *gomatrix.Client) []types.Command {
	return []types.Command{
		{
			Path: []string{"weather", "help"},
			Command: func(roomID, userID string, args []string) (interface{}, error) {
				return usageMessage(), nil
			},
		},
		{
			Path: []string{"weather"},
			Command: func(roomID, userID string, args []string) (interface{}, error) {
				return s.search(client, roomID, userID, args)
			},
		},
		{
			Path: []string{"w"},
			Command: func(roomID, userID string, args []string) (interface{}, error) {
				return s.search(client, roomID, userID, args)
			},
		},
	}
}

// usageMessage returns a matrix TextMessage representation of the service usage
func usageMessage() *gomatrix.TextMessage {
	return &gomatrix.TextMessage{
		MsgType: "m.notice",
		Body:    `Usage: !weather (city[,country])|(postal code[,country])`,
	}
}

func (s *Service) search(client *gomatrix.Client, roomID, userID string, args []string) (interface{}, error) {
	if len(args) < 1 {
		return usageMessage(), nil
	}

	u, err := url.Parse(apiBase)
	if err != nil {
		return nil, fmt.Errorf("could not parse base url: %w", err)
	}

	// TODO: investigate
	q := u.Query()
	q.Add("q", args[0])
	q.Add("appid", s.APIKey)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create http request: %w", err)
	}

	resp, err := client.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making weather request: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		return nil, fmt.Errorf("invalid response: %s", string(b))
	}

	var body weatherResponse

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("invalid weather response: %w", err)
	}

	return gomatrix.TextMessage{
		MsgType: "m.message",
		Body:    spew.Sdump(body),
	}, nil
}

// Initialise the service
func init() {
	types.RegisterService(func(serviceID, serviceUserID, webhookEndpointURL string) types.Service {
		return &Service{
			DefaultService: types.NewDefaultService(serviceID, serviceUserID, ServiceType),
		}
	})
}
