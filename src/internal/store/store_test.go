package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type fakeStorage struct {
	files    map[string][]byte
	readErr  error
	writeErr error
	writes   int
}

func (f *fakeStorage) Read(_ context.Context, key string) ([]byte, bool, error) {
	if f.readErr != nil {
		return nil, false, f.readErr
	}
	data, present := f.files[key]
	if !present {
		return nil, false, nil
	}
	return append([]byte(nil), data...), true, nil
}

func (f *fakeStorage) Write(_ context.Context, key string, data []byte) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.writes++
	if f.files == nil {
		f.files = make(map[string][]byte)
	}
	f.files[key] = append([]byte(nil), data...)
	return nil
}

// fakeWith returns storage holding one document under key.
func fakeWith(key string, data []byte) *fakeStorage {
	return &fakeStorage{files: map[string][]byte{key: append([]byte(nil), data...)}}
}

func newTestStore(storage *fakeStorage, now time.Time) *Store {
	return &Store{storage: storage, now: func() time.Time { return now }}
}

func TestSaveListAndDelete(t *testing.T) {
	ctx := context.Background()
	storage := &fakeStorage{}
	fixed := time.Date(2026, 11, 25, 10, 14, 0, 0, time.UTC)
	store := newTestStore(storage, fixed)

	saved, err := store.Save(ctx, "  今日  ", []string{"#aabbcc", "#112233"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "今日" {
		t.Fatalf("name = %q", saved.Name)
	}
	if saved.ID == "" {
		t.Fatal("empty id")
	}
	if saved.CreatedAt != fixed.Format(time.RFC3339) {
		t.Fatalf("createdAt = %q", saved.CreatedAt)
	}
	if len(saved.Colors) != 2 || saved.Colors[0] != "#AABBCC" {
		t.Fatalf("colours = %#v", saved.Colors)
	}

	// A second store over the same storage sees the write: the list lives in
	// the document, not in this process.
	again := newTestStore(storage, fixed)
	list, err := again.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != saved.ID || list[0].Name != "今日" {
		t.Fatalf("list = %#v", list)
	}

	if err := again.Delete(ctx, saved.ID); err != nil {
		t.Fatal(err)
	}
	list, err = again.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("list after delete = %#v", list)
	}
}

func TestListIsNewestFirst(t *testing.T) {
	ctx := context.Background()
	storage := &fakeStorage{}
	clock := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	current := clock
	store := &Store{storage: storage, now: func() time.Time { return current }}

	if _, err := store.Save(ctx, "第一", []string{"#111111", "#222222"}); err != nil {
		t.Fatal(err)
	}
	current = clock.Add(time.Minute)
	if _, err := store.Save(ctx, "第二", []string{"#333333", "#444444"}); err != nil {
		t.Fatal(err)
	}
	list, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "第二" || list[1].Name != "第一" {
		t.Fatalf("order = %#v", list)
	}
}

func TestIdsStayUniqueWithinOneClockReading(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(&fakeStorage{}, time.Unix(0, 0))
	first, err := store.Save(ctx, "一", []string{"#111111", "#222222"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Save(ctx, "二", []string{"#333333", "#444444"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("both palettes are %q", first.ID)
	}
}

func TestSaveValidation(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(&fakeStorage{}, time.Unix(0, 0))
	tooMany := make([]string, 0, 13)
	for index := 0; index < 13; index++ {
		tooMany = append(tooMany, fmt.Sprintf("#%06X", index+1))
	}
	cases := []struct {
		name   string
		label  string
		colors []string
	}{
		{"", "", []string{"#111111", "#222222"}},
		{"   ", "", []string{"#111111", "#222222"}},
		{strings.Repeat("长", MaxNameRunes+1), "long name", []string{"#111111", "#222222"}},
		{"控制\x00字符", "control", []string{"#111111", "#222222"}},
		{"一个", "one colour", []string{"#111111"}},
		{"重复", "duplicates", []string{"#111111", "#111111"}},
		{"无效", "invalid", []string{"#111111", "blue"}},
		{"太多", "too many", tooMany},
	}
	for _, c := range cases {
		if _, err := store.Save(ctx, c.name, c.colors); err == nil {
			t.Fatalf("%s: accepted", c.label)
		}
	}
	if _, err := store.Save(ctx, "刚好", []string{"#111111", "#222222"}); err != nil {
		t.Fatalf("a valid palette was refused: %v", err)
	}
}

func TestSaveStopsAtTheCap(t *testing.T) {
	ctx := context.Background()
	palettes := make([]Palette, 0, MaxPalettes)
	for index := 0; index < MaxPalettes; index++ {
		palettes = append(palettes, Palette{
			ID: "p" + strings.Repeat("x", index+1), Name: "旧", Colors: []string{"#111111", "#222222"},
			CreatedAt: "2026-11-25T10:00:00Z",
		})
	}
	data, err := json.Marshal(document{Version: documentVersion, Palettes: palettes})
	if err != nil {
		t.Fatal(err)
	}
	store := newTestStore(fakeWith(Key, data), time.Unix(0, 0))
	if _, err := store.Save(ctx, "新的", []string{"#111111", "#222222"}); err == nil {
		t.Fatal("saved past the cap")
	}
}

func TestCorruptDocumentIsRefusedNotOverwritten(t *testing.T) {
	ctx := context.Background()
	storage := fakeWith(Key, []byte("{not json"))
	store := newTestStore(storage, time.Unix(0, 0))
	if _, err := store.List(ctx); err == nil {
		t.Fatal("listed a corrupt document")
	}
	if _, err := store.Save(ctx, "新的", []string{"#111111", "#222222"}); err == nil {
		t.Fatal("overwrote a corrupt document")
	}
	if storage.writes != 0 {
		t.Fatalf("wrote %d times", storage.writes)
	}
}

func TestFutureVersionIsRefused(t *testing.T) {
	ctx := context.Background()
	storage := fakeWith(Key, []byte(`{"version":99,"palettes":[]}`))
	if _, err := newTestStore(storage, time.Unix(0, 0)).List(ctx); err == nil {
		t.Fatal("read a document from a newer version")
	}
}

func TestStorageErrorsReachTheCaller(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(&fakeStorage{readErr: errors.New("未授权")}, time.Unix(0, 0))
	if _, err := store.List(ctx); err == nil || !strings.Contains(err.Error(), "未授权") {
		t.Fatalf("err = %v", err)
	}
	store = newTestStore(&fakeStorage{writeErr: errors.New("配额已满")}, time.Unix(0, 0))
	if _, err := store.Save(ctx, "名字", []string{"#111111", "#222222"}); err == nil ||
		!strings.Contains(err.Error(), "配额已满") {
		t.Fatalf("err = %v", err)
	}
}

func TestCurrentMemoryStartsEmpty(t *testing.T) {
	ctx := context.Background()
	current, present, err := newTestStore(&fakeStorage{}, time.Unix(0, 0)).Current(ctx)
	if err != nil || present || current.Hex != "" {
		t.Fatalf("current = %#v present=%v err=%v", current, present, err)
	}
}

func TestRememberNormalizesAndSurvivesTheProcess(t *testing.T) {
	ctx := context.Background()
	storage := &fakeStorage{}
	fixed := time.Date(2026, 11, 25, 10, 14, 0, 0, time.UTC)
	store := newTestStore(storage, fixed)

	remembered, err := store.Remember(ctx, " abc ")
	if err != nil {
		t.Fatal(err)
	}
	if remembered.Hex != "#AABBCC" || remembered.At != fixed.Format(time.RFC3339) {
		t.Fatalf("remembered = %#v", remembered)
	}

	// A second store over the same private directory reads the same memory.
	current, present, err := newTestStore(storage, fixed).Current(ctx)
	if err != nil || !present {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if current != remembered {
		t.Fatalf("current = %#v, want %#v", current, remembered)
	}
}

func TestRememberRefusesAnInvalidColour(t *testing.T) {
	ctx := context.Background()
	storage := &fakeStorage{}
	if _, err := newTestStore(storage, time.Unix(0, 0)).Remember(ctx, "#12345"); err == nil {
		t.Fatal("remembered an invalid colour")
	}
	if storage.writes != 0 {
		t.Fatalf("wrote %d times", storage.writes)
	}
}

func TestADamagedMemoryIsAbsentNotFatal(t *testing.T) {
	ctx := context.Background()
	for name, data := range map[string]string{
		"corrupt":  "{not json",
		"future":   `{"version":99,"hex":"#AABBCC"}`,
		"no hex":   `{"version":1,"hex":"nope"}`,
		"no field": `{"version":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			storage := fakeWith(CurrentKey, []byte(data))
			if _, present, err := newTestStore(storage, time.Unix(0, 0)).Current(ctx); err != nil || present {
				t.Fatalf("present=%v err=%v", present, err)
			}
			// The next pick replaces the unusable memory instead of failing.
			if _, err := newTestStore(storage, time.Unix(0, 0)).Remember(ctx, "#112233"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCurrentStorageErrorsReachTheCaller(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(&fakeStorage{readErr: errors.New("未授权")}, time.Unix(0, 0))
	if _, _, err := store.Current(ctx); err == nil || !strings.Contains(err.Error(), "未授权") {
		t.Fatalf("err = %v", err)
	}
	store = newTestStore(&fakeStorage{writeErr: errors.New("配额已满")}, time.Unix(0, 0))
	if _, err := store.Remember(ctx, "#112233"); err == nil || !strings.Contains(err.Error(), "配额已满") {
		t.Fatalf("err = %v", err)
	}
}
