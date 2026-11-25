package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mypaste/com.mypaste.colors/src/internal/store"
)

type memoryStorage struct {
	data    []byte
	present bool
}

func (m *memoryStorage) Read(context.Context, string) ([]byte, bool, error) {
	if !m.present {
		return nil, false, nil
	}
	return append([]byte(nil), m.data...), true, nil
}

func (m *memoryStorage) Write(_ context.Context, _ string, data []byte) error {
	m.present = true
	m.data = append([]byte(nil), data...)
	return nil
}

func invocationOf(text string) invocation {
	return invocation{
		ID: "1", Method: "invoke", CommandID: commandRequest,
		Input: &requestInput{Text: text},
	}
}

func parseAnswer(t *testing.T, output string) answer {
	t.Helper()
	var reply answer
	if err := json.Unmarshal([]byte(output), &reply); err != nil {
		t.Fatalf("answer is not JSON: %v (%q)", err, output)
	}
	return reply
}

func TestDailyRequest(t *testing.T) {
	output, err := run(context.Background(), store.New(&memoryStorage{}),
		invocationOf(`{"action":"daily","date":"2026-11-25"}`))
	if err != nil {
		t.Fatal(err)
	}
	reply := parseAnswer(t, output)
	if !reply.OK || reply.Day == nil {
		t.Fatalf("%#v", reply)
	}
	if reply.Day.Date != "2026-11-25" || len(reply.Day.Swatches) != 5 {
		t.Fatalf("day = %#v", reply.Day)
	}
}

func TestMonthRequest(t *testing.T) {
	output, err := run(context.Background(), store.New(&memoryStorage{}),
		invocationOf(`{"action":"month","date":"2026-11-25"}`))
	if err != nil {
		t.Fatal(err)
	}
	reply := parseAnswer(t, output)
	if !reply.OK || reply.Month == nil {
		t.Fatalf("%#v", reply)
	}
	if reply.Month.Month != "2026-11" || len(reply.Month.Days) != 30 {
		t.Fatalf("month = %#v", reply.Month)
	}
	if last := reply.Month.Days[29]; last.Date != "2026-11-30" || len(last.Swatches) != 5 {
		t.Fatalf("last day = %#v", last)
	}
}

func TestRampRequest(t *testing.T) {
	output, err := run(context.Background(), store.New(&memoryStorage{}),
		invocationOf(`{"action":"ramp","hex":"#3A7BFF","count":9}`))
	if err != nil {
		t.Fatal(err)
	}
	reply := parseAnswer(t, output)
	if !reply.OK || len(reply.Swatches) != 9 || reply.Swatches[4].Hex != "#3A7BFF" {
		t.Fatalf("%#v", reply)
	}
}

func TestSaveListAndDelete(t *testing.T) {
	ctx := context.Background()
	storage := &memoryStorage{}
	palettes := store.New(storage)

	output, err := run(ctx, palettes,
		invocationOf(`{"action":"save","name":"今日","colors":["#AABBCC","#112233"]}`))
	if err != nil {
		t.Fatal(err)
	}
	saved := parseAnswer(t, output)
	if !saved.OK || saved.Palette == nil || saved.Palette.ID == "" {
		t.Fatalf("%#v", saved)
	}
	if len(saved.Palette.Swatches) != 2 || saved.Palette.Swatches[0].RGB == "" {
		t.Fatalf("palette = %#v", saved.Palette)
	}

	output, err = run(ctx, palettes, invocationOf(`{"action":"list"}`))
	if err != nil {
		t.Fatal(err)
	}
	listed := parseAnswer(t, output)
	if !listed.OK || len(listed.Palettes) != 1 || listed.Palettes[0].ID != saved.Palette.ID {
		t.Fatalf("%#v", listed)
	}

	output, err = run(ctx, palettes, invocationOf(
		`{"action":"delete","id":"`+saved.Palette.ID+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	deleted := parseAnswer(t, output)
	if !deleted.OK || deleted.ID != saved.Palette.ID {
		t.Fatalf("%#v", deleted)
	}

	output, err = run(ctx, palettes, invocationOf(`{"action":"list"}`))
	if err != nil {
		t.Fatal(err)
	}
	if listed = parseAnswer(t, output); len(listed.Palettes) != 0 {
		t.Fatalf("%#v", listed)
	}
}

func TestValidationRefusalsAreAnsweredNotRaised(t *testing.T) {
	cases := []string{
		`{"action":"save","name":"","colors":["#AABBCC","#112233"]}`,
		`{"action":"save","name":"名字","colors":["#AABBCC"]}`,
		`{"action":"delete","id":"missing"}`,
		`{"action":"daily","date":"today"}`,
		`{"action":"ramp","hex":"blue"}`,
		`{"action":"nope"}`,
	}
	for _, text := range cases {
		output, err := run(context.Background(), store.New(&memoryStorage{}), invocationOf(text))
		if err != nil {
			t.Fatalf("%s: raised %v", text, err)
		}
		reply := parseAnswer(t, output)
		if reply.OK || reply.Error == "" {
			t.Fatalf("%s: %#v", text, reply)
		}
	}
}

func TestRunRefusesAForeignCommand(t *testing.T) {
	call := invocation{ID: "1", Method: "invoke", CommandID: "other.command",
		Input: &requestInput{Text: `{"action":"list"}`}}
	if _, err := run(context.Background(), store.New(&memoryStorage{}), call); err == nil {
		t.Fatal("accepted a foreign command")
	}
	call.Method = "trigger"
	call.CommandID = commandRequest
	if _, err := run(context.Background(), store.New(&memoryStorage{}), call); err == nil {
		t.Fatal("accepted a non-invoke method")
	}
}

func TestRunRefusesAMalformedBody(t *testing.T) {
	for _, text := range []string{"", "not json"} {
		if _, err := run(context.Background(), store.New(&memoryStorage{}),
			invocationOf(text)); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
	call := invocation{ID: "1", Method: "invoke", CommandID: commandRequest}
	if _, err := run(context.Background(), store.New(&memoryStorage{}), call); err == nil ||
		!strings.Contains(err.Error(), "请求") {
		t.Fatalf("err = %v", err)
	}
}
