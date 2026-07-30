package content

import "gopkg.in/yaml.v3"

type Menu struct {
	Title    string    `yaml:"title"`
	Note     string    `yaml:"note"`
	Sections []Section `yaml:"sections"`
}

type Section struct {
	Name  string `yaml:"name"`
	Items []Item `yaml:"items"`
}

type Item struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Ingredients []string `yaml:"ingredients"`
	Tags        []string `yaml:"tags"`
	Price       string   `yaml:"price"`
	Featured    bool     `yaml:"featured"`
}

type MenuLoader struct {
	cache *Cache
}

func NewMenuLoader(cache *Cache) *MenuLoader {
	return &MenuLoader{cache: cache}
}

func (l *MenuLoader) Load(path string) (Menu, error) {
	v, err := l.cache.Get(path, func(b []byte) (any, error) {
		var m Menu
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		return m, nil
	})
	if err != nil {
		if m, ok := v.(Menu); ok {
			return m, err
		}
		return Menu{}, err
	}
	return v.(Menu), nil
}
