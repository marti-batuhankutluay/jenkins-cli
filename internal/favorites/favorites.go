package favorites

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Favorite struct {
	Name    string `yaml:"name"`
	JobPath string `yaml:"job_path"`
	EnvName string `yaml:"env_name"`
}

// ToggleFavoriteMsg is a bubbletea message emitted from any screen to toggle a favorite.
type ToggleFavoriteMsg struct {
	Fav Favorite
}

// FavToggledMsg is sent back to the originating screen after a toggle completes.
type FavToggledMsg struct {
	Added bool
	Name  string
}

type Favorites struct {
	Items []Favorite `yaml:"favorites"`
}

func filePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "jenkins-cli", "favorites.yaml"), nil
}

func Load() (*Favorites, error) {
	path, err := filePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Favorites{}, nil
		}
		return nil, fmt.Errorf("reading favorites: %w", err)
	}
	var f Favorites
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing favorites: %w", err)
	}
	return &f, nil
}

func save(f *Favorites) error {
	path, err := filePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshaling favorites: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

func (f *Favorites) Has(jobPath string) bool {
	for _, item := range f.Items {
		if item.JobPath == jobPath {
			return true
		}
	}
	return false
}

func (f *Favorites) Add(fav Favorite) error {
	if f.Has(fav.JobPath) {
		return nil
	}
	f.Items = append(f.Items, fav)
	return save(f)
}

func (f *Favorites) Remove(jobPath string) error {
	items := f.Items[:0]
	for _, item := range f.Items {
		if item.JobPath != jobPath {
			items = append(items, item)
		}
	}
	f.Items = items
	return save(f)
}

func (f *Favorites) Toggle(fav Favorite) (added bool, err error) {
	if f.Has(fav.JobPath) {
		return false, f.Remove(fav.JobPath)
	}
	return true, f.Add(fav)
}
