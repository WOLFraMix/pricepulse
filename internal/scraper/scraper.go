package scraper

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"gopkg.in/yaml.v3"
)

// Profile — «паспорт» магазина: как вытащить из его страницы цену и название.
// Новые магазины добавляются в sources.yaml, а не в код — это принцип,
// который спасает от ветки if в каждом новом месте.
type Profile struct {
	Name          string `yaml:"name"`
	PriceSelector string `yaml:"price_selector"`
	TitleSelector string `yaml:"title_selector"`
	Currency      string `yaml:"currency"`
}

type Config struct {
	Sources map[string]Profile `yaml:"sources"`
}

// LoadProfiles читает YAML-конфиг с профилями.
func LoadProfiles(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if len(cfg.Sources) == 0 {
		return Config{}, fmt.Errorf("no sources in %s", path)
	}
	return cfg, nil
}

type Scraper struct {
	client   *http.Client
	profiles Config
}

func New(cfg Config) *Scraper {
	return &Scraper{
		client:   &http.Client{Timeout: 20 * time.Second},
		profiles: cfg,
	}
}

// digits вырезает из текста вида «2 990 ₽» или «£51.77» первое число.
var digits = regexp.MustCompile(`[0-9]+(?:[.,][0-9]+)?`)

// parsePrice превращает текст цены в число. Нюансы: пробелы-разделители
// тысяч («2 990»), запятая как десятичный разделитель, символ валюты.
func parsePrice(s string) (float64, error) {
	m := digits.FindString(s)
	if m == "" {
		return 0, fmt.Errorf("price not found in %q", s)
	}
	m = strings.ReplaceAll(m, ",", ".")
	return strconv.ParseFloat(m, 64)
}

// Check скачивает страницу и вытаскивает цену и название по профилю.
func (s *Scraper) Check(ctx context.Context, url, source string) (price float64, title, currency string, err error) {
	p, ok := s.profiles.Sources[source]
	if !ok {
		return 0, "", "", fmt.Errorf("unknown source %q", source)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "", "", err
	}
	// Вежливый и честный User-Agent: указываем, кто мы. Сайты,
	// которые режут ботов, отрежут и нас — и это корректно.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; PricePulse/1.0; personal price tracker)")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, "", "", fmt.Errorf("http %d from %s", resp.StatusCode, url)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return 0, "", "", err
	}

	// Find возвращает выборку по CSS-селектору (через cascadia),
	// First берёт первый элемент, Text — текстовое содержимое.
	title = strings.TrimSpace(doc.Find(p.TitleSelector).First().Text())
	priceText := strings.TrimSpace(doc.Find(p.PriceSelector).First().Text())

	price, err = parsePrice(priceText)
	if err != nil {
		return 0, title, p.Currency, fmt.Errorf("selector %q: %w", p.PriceSelector, err)
	}
	return price, title, p.Currency, nil
}
