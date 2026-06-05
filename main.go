//go:build wasip1

package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/glasslabs/client-go"
)

//go:embed  assets/icons
var iconFS embed.FS

const (
	api             = "https://api.openweathermap.org/data/2.5/"
	apiCurrentPath  = "weather"
	apiForecastPath = "forecast/daily"
)

// Config is the module configuration.
type Config struct {
	LocationID string `json:"locationId"`
	AppID      string `json:"appId"`
	Units      string `json:"units"`
	Interval   string `json:"interval"`
}

// NewConfig returns a Config with default values set.
func NewConfig() Config {
	return Config{Interval: "30m"}
}

var (
	mod  *client.Module
	cfg  Config
	svgs map[string]string
	log  *client.Logger
)

// iconFiles maps OWM icon codes to loaded SVG keys ("day/<name>" or "night/<name>").
var iconFiles = map[string]string{
	"01d": "day/clear", "01n": "night/clear",
	"02d": "day/mostlysunny", "02n": "night/mostlysunny",
	"03d": "day/partlycloudy", "03n": "night/partlycloudy",
	"04d": "day/cloudy", "04n": "night/cloudy",
	"09d": "day/chancerain", "09n": "night/chancerain",
	"10d": "day/rain", "10n": "night/rain",
	"11d": "day/tstorms", "11n": "night/tstorms",
	"13d": "day/snow", "13n": "night/snow",
	"50d": "day/fog", "50n": "night/fog",
}

func main() {
	log = client.NewLogger()

	var err error
	mod, err = client.NewModule()
	if err != nil {
		log.Error("Could not create module", "error", err.Error())
		return
	}

	cfg = NewConfig()
	if err = mod.ParseConfig(&cfg); err != nil {
		log.Error("Could not parse config", "error", err.Error())
		return
	}

	loadSVGs()

	log.Info("Module ready", "module", mod.Name())

	update()

	interval := 30 * time.Minute
	if d, err := time.ParseDuration(cfg.Interval); err == nil {
		interval = d
	}

	for {
		time.Sleep(interval)
		update()
	}
}

// loadSVGs reads all weather icon SVG files from the embedded assets into memory.
func loadSVGs() {
	svgs = make(map[string]string)
	names := []string{
		"chanceflurries", "chancerain", "chancesleet", "chancesnow", "chancetstorms",
		"clear", "cloudy", "flurries", "fog", "hazy", "mostlycloudy", "mostlysunny",
		"partlycloudy", "partlysunny", "rain", "sleet", "snow", "sunny", "tstorms", "unknown",
	}
	for _, name := range names {
		for _, tod := range []string{"day", "night"} {
			b, err := iconFS.ReadFile("assets/icons/" + tod + "/" + name + ".svg")
			if err != nil {
				log.Error("Could not load icon", "icon", tod+"/"+name, "error", err.Error())
				continue
			}
			svgs[tod+"/"+name] = string(b)
		}
	}
}

// iconWidget returns an SVG widget for the given OWM icon code at the requested dp size.
func iconWidget(code string, size int) *client.SVG {
	key, ok := iconFiles[code]
	if !ok {
		key = "day/unknown"
	}
	content, ok := svgs[key]
	if !ok {
		content = svgs["day/unknown"]
	}
	return client.NewSVG(withSVGSize(content, size))
}

// withSVGSize injects explicit width and height attributes into an SVG string.
func withSVGSize(content string, size int) string {
	attrs := fmt.Sprintf(`width="%d" height="%d" `, size, size)
	return strings.Replace(content, "<svg", "<svg "+attrs, 1)
}

func update() {
	d := data{}
	if err := apiRequest(apiCurrentPath, url.Values{}, &d.Current); err != nil {
		log.Error("Could not get current weather", "error", err.Error())
	}
	if err := apiRequest(apiForecastPath, url.Values{"cnt": {"5"}}, &d.Forecast); err != nil {
		log.Error("Could not get weather forecast", "error", err.Error())
	}

	if len(d.Forecast.List) > 1 {
		d.Current.Day = d.Forecast.List[0]
		d.Forecast.List = d.Forecast.List[1:]
	}
	d.Current.Icon = d.Current.Weather.code()
	for i := range d.Forecast.List {
		dy := d.Forecast.List[i]
		dy.Day = time.Unix(dy.Unix, 0).Format("Mon")
		dy.Icon = dy.Weather.code()
		d.Forecast.List[i] = dy
	}

	render(d)
}

const forecastDays = 4

func render(d data) {
	unitSuffix := "°C"
	if cfg.Units == "imperial" {
		unitSuffix = "°F"
	}

	// Current conditions: always show full layout; zero data and unknown icon when missing.
	currentWidget := client.NewHStack(
		iconWidget(d.Current.Icon, 80),
		client.NewSpacer(client.WithMinSize(8)),
		client.NewText(
			fmt.Sprintf("%.0f", d.Current.Main.Temp),
			client.WithColor("#ffffff"),
			client.WithFontSize(68),
			client.WithLight(),
		),
		client.NewText(
			unitSuffix,
			client.WithColor("#cccccc"),
			client.WithFontSize(30),
			client.WithLight(),
		),
		client.NewSpacer(client.WithMinSize(16)),
		client.NewTable([]*client.Row{
			{
				Columns: []*client.Column{
					client.NewColumn(client.NewText(
						"Max",
						client.WithColor("#ffffff"),
						client.WithFontSize(20),
					), 0),
					client.NewColumn(client.NewText(
						" : ",
						client.WithColor("#cccccc"),
						client.WithFontSize(20),
					), 0),
					client.NewColumn(client.NewText(
						fmt.Sprintf("%.0f°", d.Current.Day.Temp.Max),
						client.WithColor("#cccccc"),
						client.WithFontSize(20),
					), 0),
				},
			},
			{
				Columns: []*client.Column{
					client.NewColumn(client.NewText(
						"Min",
						client.WithColor("#ffffff"),
						client.WithFontSize(20),
					), 0),
					client.NewColumn(client.NewText(
						" : ",
						client.WithColor("#cccccc"),
						client.WithFontSize(20),
					), 0),
					client.NewColumn(client.NewText(
						fmt.Sprintf("%.0f°", d.Current.Day.Temp.Min),
						client.WithColor("#cccccc"),
						client.WithFontSize(20),
					), 0),
				},
			},
			{
				Columns: []*client.Column{
					client.NewColumn(client.NewText(
						"Rain",
						client.WithColor("#ffffff"),
						client.WithFontSize(20),
					), 0),
					client.NewColumn(client.NewText(
						" : ",
						client.WithColor("#cccccc"),
						client.WithFontSize(20),
					), 0),
					client.NewColumn(client.NewText(
						fmt.Sprintf("%.0f mm", d.Current.Day.Rain),
						client.WithColor("#cccccc"),
						client.WithFontSize(20),
					), 0),
				},
			},
		}),
	)

	slots := make([]day, forecastDays)
	copy(slots, d.Forecast.List)

	forecastItems := make([]client.Widget, 0, forecastDays*2-1)
	for i, dy := range slots {
		if i > 0 {
			forecastItems = append(forecastItems, client.NewSpacer())
		}
		dayLabel := dy.Day
		if dayLabel == "" {
			dayLabel = "—"
		}
		forecastItems = append(forecastItems, client.NewVStack(
			client.NewText(dayLabel,
				client.WithColor("#aaaaaa"),
				client.WithFontSize(16),
				client.WithAlign("center"),
			),
			iconWidget(dy.Icon, 60),
			client.NewText(
				fmt.Sprintf("%.0f° / %.0f°", dy.Temp.Max, dy.Temp.Min),
				client.WithColor("#cccccc"),
				client.WithFontSize(16),
				client.WithAlign("center"),
			),
		))
	}

	mod.Render(client.NewVStack(
		currentWidget,
		client.NewSpacer(client.WithMinSize(10)),
		client.NewHStack(forecastItems...),
	))
}

func apiRequest(p string, qry url.Values, v any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	u, err := url.Parse(api + p)
	if err != nil {
		return fmt.Errorf("parsing url: %w", err)
	}
	q := url.Values{}
	q.Set("id", cfg.LocationID)
	q.Set("appid", cfg.AppID)
	q.Set("units", cfg.Units)
	maps.Copy(q, qry)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("requesting url: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		de := dataError{}
		if err = json.NewDecoder(resp.Body).Decode(&de); err != nil {
			return fmt.Errorf("parsing error response: %w", err)
		}
		return fmt.Errorf("API error: %s", de.Message)
	}

	if err = json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	return nil
}

type dataError struct {
	Code    int    `json:"cod"`
	Message string `json:"message"`
}

type data struct {
	Current  current
	Forecast forecast
}

type current struct {
	Main struct {
		Temp float64 `json:"temp"`
	} `json:"main"`
	Day     day
	Weather weather `json:"weather"`
	Icon    string
}

type forecast struct {
	List []day `json:"list"`
}

type day struct {
	Unix int64 `json:"dt"`
	Day  string
	Temp struct {
		Min float64 `json:"min"`
		Max float64 `json:"max"`
	} `json:"temp"`
	Weather weather `json:"weather"`
	Icon    string
	Rain    float64 `json:"rain"`
}

type weather []struct {
	IconCode string `json:"icon"`
}

// code returns the OWM icon code for the first weather condition, or an empty string.
func (w weather) code() string {
	if len(w) == 0 {
		return ""
	}
	return w[0].IconCode
}
