package model

// Generic Bookmark Data Model
type Bookmark struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"link"`
	LocalURL string `yaml:"local_link,omitempty"`
	Icon     string `yaml:"icon,omitempty"`
	Desc     string `yaml:"desc,omitempty"`
	Private  bool   `yaml:"private,omitempty"`
	Favorite bool   `yaml:"favorite,omitempty"`
	Category string `yaml:"category,omitempty"`
	Subdir   string `yaml:"subdir,omitempty"`
}

// Generic Category Data Model
type Category struct {
	ID   string `yaml:"id"`
	Name string `yaml:"title"`
}

// Generic Bookmarks Data Model
type Bookmarks struct {
	Categories []Category `yaml:"categories,omitempty"`
	Items      []Bookmark `yaml:"links"`
}
