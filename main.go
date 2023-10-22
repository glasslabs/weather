//go:build js && wasm

package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/glasslabs/client-go"
)

const (
	api             = "https://api.openweathermap.org/data/2.5/"
	apiCurrentPath  = "weather"
	apiForecastPath = "forecast/daily"
)

var (
	//go:embed assets/style.css
	css []byte

	//go:embed assets/wu-icons-style.css
	icons []byte

	//go:embed assets/index.html
	html []byte
)

// Config is the module configuration.
type Config struct {
	LocationID string        `yaml:"locationId"`
	AppID      string        `yaml:"appId"`
	Units      string        `yaml:"units"`
	Interval   time.Duration `yaml:"interval"`
}

// NewConfig returns a Config with default values set.
func NewConfig() Config {
	return Config{
		Interval: 30 * time.Minute,
	}
}

func main() {
	log := client.NewLogger()
	mod, err := client.NewModule()
	if err != nil {
		log.Error("Could not create module", "error", err.Error())
		return
	}

	cfg := NewConfig()
	if err = mod.ParseConfig(&cfg); err != nil {
		log.Error("Could not parse config", "error", err.Error())
		return
	}

	log.Info("Loading Module", "module", mod.Name())

	m := &Module{
		mod: mod,
		cfg: cfg,
		log: log,
	}

	if err = m.setup(); err != nil {
		log.Error("Could not setup module", "error", err.Error())
		return
	}

	tick := time.NewTicker(cfg.Interval)
	defer tick.Stop()

	for {
		m.update()

		<-tick.C
	}
}

// Module runs the module.
type Module struct {
	mod *client.Module
	cfg Config

	tmpl *template.Template

	log *client.Logger
}

func (m *Module) setup() error {
	tmpl, err := template.New("html").Parse(string(html))
	if err != nil {
		return fmt.Errorf("paring template: %w", err)
	}
	m.tmpl = tmpl

	if err = m.mod.LoadCSS(string(css), string(icons)); err != nil {
		return fmt.Errorf("loading css: %w", err)
	}

	if err = m.render(data{}); err != nil {
		m.log.Error("Could not render weather data", "error", err.Error())
	}
	return nil
}

func (m *Module) update() {
	d := data{}
	if err := m.request(apiCurrentPath, url.Values{}, &d.Current); err != nil {
		m.log.Error("Could not get current weather data", "error", err.Error())
	}
	if err := m.request(apiForecastPath, url.Values{"cnt": []string{"4"}}, &d.Forecast); err != nil {
		m.log.Error("Could not get current weather data", "error", err.Error())
	}

	if len(d.Forecast.List) > 1 {
		d.Current.Day = d.Forecast.List[0]
		d.Forecast.List = d.Forecast.List[1:]
	}
	d.Current.Icon = d.Current.Weather.Icon()
	for i := range d.Forecast.List {
		dy := d.Forecast.List[i]

		t := time.Unix(dy.Unix, 0)
		dy.Day = t.Format("Monday")
		dy.Icon = dy.Weather.Icon()

		d.Forecast.List[i] = dy
	}

	if err := m.render(d); err != nil {
		m.log.Error("Could not render weather data", "error", err.Error())
	}
}

func (m *Module) render(d data) error {
	var buf bytes.Buffer
	if err := m.tmpl.Execute(&buf, d); err != nil {
		return fmt.Errorf("rendering html: %w", err)
	}
	m.mod.Element().SetInnerHTML(buf.String())
	return nil
}

func (m *Module) request(p string, qry url.Values, v interface{}) error {
	u, err := url.Parse(api + p)
	if err != nil {
		return fmt.Errorf("could not parse url: %w", err)
	}
	q := url.Values{}
	q.Set("id", m.cfg.LocationID)
	q.Set("appid", m.cfg.AppID)
	q.Set("units", m.cfg.Units)
	for k, val := range qry {
		q[k] = val
	}
	u.RawQuery = q.Encode()

	//nolint:noctx
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("could create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not request url: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		de := dataError{}
		if err = json.NewDecoder(resp.Body).Decode(&de); err != nil {
			return fmt.Errorf("could not parse error: %w", err)
		}
		return fmt.Errorf("could not fetch data: %s", de.Message)
	}

	if err = json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("could not parse data: %w", err)
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

const unknownIcon = "wu-unknown"

var iconTable = map[string]string{
	"01d": "wu-clear",
	"02d": "wu-partlycloudy",
	"03d": "wu-cloudy",
	"04d": "wu-cloudy",
	"09d": "wu-flurries",
	"10d": "wu-rain",
	"11d": "wu-tstorms",
	"13d": "wu-snow",
	"50d": "wu-fog",
	"01n": "wu-clear wu-night",
	"02n": "wu-partlycloudy wu-night",
	"03n": "wu-cloudy wu-night",
	"04n": "wu-cloudy wu-night",
	"09n": "wu-flurries wu-night",
	"10n": "wu-rain wu-night",
	"11n": "wu-tstorms wu-night",
	"13n": "wu-snow wu-night",
	"50n": "wu-fog wu-night",
}

type weather []struct {
	IconCode string `json:"icon"`
}

// Icon returns the weather icon or the unknown icon.
func (w weather) Icon() string {
	if len(w) == 0 {
		return unknownIcon
	}
	icn, ok := iconTable[w[0].IconCode]
	if !ok {
		return unknownIcon
	}
	return icn
}
