// Package store keeps the reader's saved palettes in the package's private
// storage. The only host service it needs is storage.plugin; everything it
// writes is a small JSON document with one entry per palette, newest first.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mypaste/com.mypaste.colors/src/internal/color"
)

const (
	// Key is the single object this extension keeps. One document means one
	// read and one write per change, and a palette list that can be copied
	// out of the plugin's private directory as-is.
	Key = "palettes.json"
	// MaxPalettes bounds the list; beyond it the page would be a scroll no
	// one finishes, so saving stops with a clear message.
	MaxPalettes = 200
	// MaxNameRunes is the reader's name, not ours to grow.
	MaxNameRunes = 40
	// A palette needs at least two colours to be a palette, and no more than
	// the page can draw as one row of swatches.
	MinColors = 2
	MaxColors = 12

	documentVersion = 1
)

// Palette is one saved palette. Colors are canonical uppercase #RRGGBB.
type Palette struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Colors    []string `json:"colors"`
	CreatedAt string   `json:"createdAt"`
}

// Storage is the part of the host's storage.plugin service this package uses.
// Keeping it an interface lets the store be tested without a host process.
type Storage interface {
	Read(ctx context.Context, key string) ([]byte, bool, error)
	Write(ctx context.Context, key string, data []byte) error
}

// Store reads and writes one document through the host.
type Store struct {
	storage Storage
	now     func() time.Time
}

// New returns a store over the host's private storage.
func New(storage Storage) *Store {
	return &Store{storage: storage, now: time.Now}
}

type document struct {
	Version  int       `json:"version"`
	Palettes []Palette `json:"palettes"`
}

// List returns every saved palette, newest first.
func (s *Store) List(ctx context.Context) ([]Palette, error) {
	doc, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Palette, len(doc.Palettes))
	copy(out, doc.Palettes)
	return out, nil
}

// Save validates the reader's palette and puts it at the top of the list.
func (s *Store) Save(ctx context.Context, name string, colors []string) (Palette, error) {
	name, err := CleanName(name)
	if err != nil {
		return Palette{}, err
	}
	colors, err = CleanColors(colors)
	if err != nil {
		return Palette{}, err
	}
	doc, err := s.load(ctx)
	if err != nil {
		return Palette{}, err
	}
	if len(doc.Palettes) >= MaxPalettes {
		return Palette{}, fmt.Errorf("最多保存 %d 套配色，先删掉一些再保存", MaxPalettes)
	}
	now := s.now()
	palette := Palette{
		ID:        s.newID(now, doc.Palettes),
		Name:      name,
		Colors:    colors,
		CreatedAt: now.Format(time.RFC3339),
	}
	doc.Palettes = append([]Palette{palette}, doc.Palettes...)
	if err := s.persist(ctx, doc); err != nil {
		return Palette{}, err
	}
	return palette, nil
}

// Delete removes one palette by the id the list returned.
func (s *Store) Delete(ctx context.Context, id string) error {
	doc, err := s.load(ctx)
	if err != nil {
		return err
	}
	found := -1
	for index := range doc.Palettes {
		if doc.Palettes[index].ID == id {
			found = index
			break
		}
	}
	if found < 0 {
		return errors.New("这套配色已经不在了")
	}
	doc.Palettes = append(doc.Palettes[:found], doc.Palettes[found+1:]...)
	return s.persist(ctx, doc)
}

// CleanName trims and validates the reader's name.
func CleanName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", errors.New("配色需要一个名字")
	}
	if utf8.RuneCountInString(name) > MaxNameRunes {
		return "", fmt.Errorf("名字最多 %d 个字", MaxNameRunes)
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return "", errors.New("名字里不能有控制字符")
		}
	}
	return name, nil
}

// CleanColors normalises each colour, drops repeats and enforces the bounds.
func CleanColors(values []string) ([]string, error) {
	colors := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		hex, ok := color.NormalizeHex(value)
		if !ok {
			return nil, errors.New("颜色值应为 #RRGGBB")
		}
		if seen[hex] {
			continue
		}
		seen[hex] = true
		colors = append(colors, hex)
	}
	if len(colors) < MinColors {
		return nil, errors.New("一套配色至少要有两种不同的颜色")
	}
	if len(colors) > MaxColors {
		return nil, fmt.Errorf("一套配色最多 %d 种颜色", MaxColors)
	}
	return colors, nil
}

func (s *Store) load(ctx context.Context) (document, error) {
	raw, present, err := s.storage.Read(ctx, Key)
	if err != nil {
		return document{}, fmt.Errorf("无法读取保存的配色：%w", err)
	}
	if !present {
		return document{Version: documentVersion, Palettes: []Palette{}}, nil
	}
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return document{}, errors.New("保存的配色已损坏，无法读取")
	}
	if doc.Version > documentVersion {
		return document{}, errors.New("保存的配色来自更新的版本，无法读取")
	}
	if doc.Palettes == nil {
		doc.Palettes = []Palette{}
	}
	return doc, nil
}

func (s *Store) persist(ctx context.Context, doc document) error {
	doc.Version = documentVersion
	data, err := json.Marshal(doc)
	if err != nil {
		return errors.New("无法编码保存的配色")
	}
	if err := s.storage.Write(ctx, Key, append(data, '\n')); err != nil {
		return fmt.Errorf("无法保存配色：%w", err)
	}
	return nil
}

func (s *Store) newID(now time.Time, existing []Palette) string {
	taken := make(map[string]bool, len(existing))
	for _, palette := range existing {
		taken[palette.ID] = true
	}
	base := "p" + strconv.FormatInt(now.UnixNano(), 36)
	id := base
	for suffix := 2; taken[id]; suffix++ {
		id = base + "-" + strconv.Itoa(suffix)
	}
	return id
}
