// Package weather implements a Service which adds !commands for openweather search
package weather

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/matrix-org/go-neb/types"
	"github.com/matrix-org/gomatrix"
)

// ServiceType of the Weather service
const ServiceType = "weather"

const apiBase = "https://api.openweathermap.org/data/2.5/weather"

var httpClient = &http.Client{}

type weatherResponse struct {
	ID         int64       `json:"id"`
	Name       string      `json:"name"`
	Coord      coord       `json:"coord"`
	Weather    []weather   `json:"weather"`
	Base       string      `json:"base"`
	Main       mainInfo    `json:"main"`
	Visibility float64     `json:"visibility"`
	Wind       wind        `json:"wind"`
	Rain       rain        `json:"rain"`
	Snow       snow        `json:"snow"`
	Dt         weatherTime `json:"dt"`
	Sys        sys         `json:"sys"`
	Timezone   int         `json:"timezone"`
}

func (w *weatherResponse) Conditions() weather {
	if len(w.Weather) > 0 {
		return w.Weather[0]
	}

	return weather{}
}

type weatherTime time.Time

func (w *weatherTime) UnmarshalJSON(b []byte) error {
	t, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return err
	}

	*w = weatherTime(time.Unix(t, 0))

	return nil
}

type sys struct {
	Country string `json:"country"`
	Sunrise int64  `json:"sunrise"`
	Sunset  int64  `json:"sunset"`
}

type snow struct {
	Hour1 float64 `json:"1h"`
	Hour3 float64 `json:"3h"`
}

type rain struct {
	Hour1 float64 `json:"1h"`
	Hour3 float64 `json:"3h"`
}

type clouds struct {
	All float64 `json:"all"`
}

type mainInfo struct {
	Temp        temp    `json:"temp"`
	FeelsLike   temp    `json:"feels_like"`
	TempMin     temp    `json:"temp_min"`
	TempMax     temp    `json:"temp_max"`
	Pressure    float64 `json:"pressure"`
	Humidity    float64 `json:"humidity"`
	SeaLevel    float64 `json:"sea_level"`
	GroundLevel float64 `json:"grnd_level"`
}

func (m mainInfo) MinMax() string {
	return fmt.Sprintf("%.2f°F / %.2f°F (%.2f°C / %.2f°C)", m.TempMax.f(), m.TempMin.f(), m.TempMax.c(), m.TempMin.c())
}

type weather struct {
	ID          int    `json:"id"`
	Main        string `json:"main"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

func (w weather) SimpleString() string {
	return fmt.Sprintf("%s (%s)", w.Main, w.Description)
}

type coord struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type temp float64

func (t temp) f() temp {
	return (t.c()*9/5 + 32)
}

func (t temp) c() temp {
	return (t - 273.15)
}

func (t temp) String() string {
	return fmt.Sprintf("%.2f°F (%.2f°C)", t.f(), t.c())
}

type speed float64

func (s speed) MPH() speed {
	return s * 0.621371
}

func (s speed) KMH() speed {
	return s
}

type deg float64

func (d deg) String() string {
	switch {
	case d > 348.75 && d <= 360.0 || d > 0 && d <= 11.25:
		return "N"
	case d > 11.25 && d <= 33.75:
		return "NNE"
	case d > 33.75 && d <= 56.25:
		return "NE"
	case d > 56.25 && d <= 78.75:
		return "ENE"
	case d > 78.75 && d <= 101.25:
		return "E"
	case d > 101.25 && d <= 123.75:
		return "ESE"
	case d > 123.75 && d <= 146.25:
		return "SE"
	case d > 146.25 && d <= 168.75:
		return "SSE"
	case d > 168.75 && d <= 191.25:
		return "S"
	case d > 191.25 && d <= 213.75:
		return "SSW"
	case d > 213.75 && d <= 236.25:
		return "SW"
	case d > 236.25 && d < 258.75:
		return "WSW"
	case d > 258.75 && d <= 281.25:
		return "W"
	case d > 281.25 && d <= 303.75:
		return "WNW"
	case d > 303.75 && d <= 326.25:
		return "NW"
	case d > 326.25 && d <= 348.75:
		return "NNW"
	default:
		return ""
	}
}

type wind struct {
	Speed speed `json:"speed"`
	Deg   deg   `json:"deg"`
}

func (w wind) String() string {
	return fmt.Sprintf("%s at %.1f MPH (%.1f km/h)", w.Deg, w.Speed.MPH(), w.Speed.KMH())
}

// Service contains the Config fields for the Weather service.
//
// Example request:
//   {
//			"api_key": "AIzaSyA4FD39..."
//   }
type Service struct {
	types.DefaultService
	APIKey         string `json:"api_key"`
	DefaultCountry string `json:"default_country"`
	Unit           string `json:"unit"`
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

	country := s.DefaultCountry
	if country == "" {
		country = "us"
	}

	argStr := strings.Join(args, " ")
	if strings.Index(argStr, ", ") == -1 {
		argStr += ", " + country
	}

	q := u.Query()
	q.Add("q", argStr)
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

	return gomatrix.HTMLMessage{
		MsgType: "m.text",
		//Format:  "org.matrix.custom.html",
		//FormattedBody: fmt.Sprintf(`<html><body><img src="data:image/png;base64,%s" width="16" height="16"></img></body></html>`, icon),
		Body: fmt.Sprintf(
			"%s || Updated: %s || Conditions: %s || Temperature: %s || High/Low: %s || Humidity: %.0f%% || %s",
			body.Name,
			humanize.Time(time.Time(body.Dt)),
			body.Conditions().SimpleString(),
			body.Main.Temp,
			body.Main.MinMax(),
			body.Main.Humidity,
			body.Wind,
		),
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
