// Command mypaste-colors is the colours extension's runtime. The host starts
// it for one request, writes one invocation line, answers the storage calls it
// makes, and reads one response line (PLUGINS.md §4). The palette a date grows
// and the tint/shade scale around a picked colour are computed here, never in
// the page; saved palettes live in the package's private storage.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/mypaste/com.mypaste.colors/src/internal/color"
	"github.com/mypaste/com.mypaste.colors/src/internal/hook"
	"github.com/mypaste/com.mypaste.colors/src/internal/store"
)

// Version is the extension version written into archive manifests.
const Version = "1.0.3"

const commandRequest = "colors.request"

// input is the text one invocation carries: the page's JSON request.
type input struct {
	Action string   `json:"action"`
	Date   string   `json:"date"`
	Hex    string   `json:"hex"`
	Count  int      `json:"count"`
	Name   string   `json:"name"`
	Colors []string `json:"colors"`
	ID     string   `json:"id"`
}

type requestInput struct {
	Text string `json:"text"`
}

type invocation struct {
	ID        string        `json:"id"`
	Method    string        `json:"method"`
	CommandID string        `json:"commandId"`
	Deadline  int64         `json:"deadlineUnixMs"`
	Input     *requestInput `json:"input,omitempty"`
}

type failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type result struct {
	Output string `json:"output,omitempty"`
}

type response struct {
	ID     string   `json:"id"`
	OK     bool     `json:"ok"`
	Result *result  `json:"result,omitempty"`
	Error  *failure `json:"error,omitempty"`
}

// paletteView is a saved palette with its colours already formatted. The
// stored document keeps only hex values; formatting is derived on the way out.
type paletteView struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	CreatedAt string         `json:"createdAt"`
	Swatches  []color.Swatch `json:"swatches"`
}

// answer carries the page's data. Its own ok and message let the page tell a
// validation refusal ("那个名字太长了") from a host failure.
type answer struct {
	OK       bool           `json:"ok"`
	Error    string         `json:"error,omitempty"`
	Day      *color.Day     `json:"day,omitempty"`
	Month    *color.Month   `json:"month,omitempty"`
	Swatches []color.Swatch `json:"swatches,omitempty"`
	Palettes []paletteView  `json:"palettes,omitempty"`
	Palette  *paletteView   `json:"palette,omitempty"`
	ID       string         `json:"id,omitempty"`
}

func main() {
	client := hook.NewClient(os.Stdin, os.Stdout)
	line, err := client.ReadRequestLine()
	if err != nil {
		os.Exit(1)
	}
	var call invocation
	if err := json.Unmarshal(line, &call); err != nil || call.ID == "" {
		os.Exit(1)
	}
	ctx, cancel := deadline(call.Deadline)
	defer cancel()
	output, runErr := run(ctx, store.New(client), call)
	if runErr != nil {
		write(client, response{ID: call.ID, OK: false,
			Error: &failure{Code: "runtime", Message: runErr.Error()}})
		return
	}
	write(client, response{ID: call.ID, OK: true, Result: &result{Output: output}})
}

func run(ctx context.Context, palettes *store.Store, call invocation) (string, error) {
	// The host writes every invocation with method "invoke" (PLUGINS.md §4);
	// the menu page's request arrives as pipe input, so its type is text/plain.
	if call.Method != "invoke" || call.CommandID != commandRequest {
		return "", errors.New("这个扩展只提供 colors.request")
	}
	if call.Input == nil || call.Input.Text == "" {
		return "", errors.New("没有收到请求内容")
	}
	var asked input
	if err := json.Unmarshal([]byte(call.Input.Text), &asked); err != nil {
		return "", errors.New("无法读取请求内容")
	}

	reply := answer{OK: true}
	switch asked.Action {
	case "daily":
		day, err := color.Daily(asked.Date)
		if err != nil {
			reply.OK, reply.Error = false, err.Error()
			break
		}
		reply.Day = &day
	case "month":
		month, err := color.MonthOf(asked.Date)
		if err != nil {
			reply.OK, reply.Error = false, err.Error()
			break
		}
		reply.Month = &month
	case "ramp":
		swatches, err := color.Ramp(asked.Hex, asked.Count)
		if err != nil {
			reply.OK, reply.Error = false, err.Error()
			break
		}
		reply.Swatches = swatches
	case "list":
		saved, err := palettes.List(ctx)
		if err != nil {
			reply.OK, reply.Error = false, err.Error()
			break
		}
		reply.Palettes = make([]paletteView, 0, len(saved))
		for _, palette := range saved {
			view, err := viewOf(palette)
			if err != nil {
				reply.OK, reply.Error = false, err.Error()
				break
			}
			reply.Palettes = append(reply.Palettes, view)
		}
	case "save":
		saved, err := palettes.Save(ctx, asked.Name, asked.Colors)
		if err != nil {
			reply.OK, reply.Error = false, err.Error()
			break
		}
		view, err := viewOf(saved)
		if err != nil {
			reply.OK, reply.Error = false, err.Error()
			break
		}
		reply.Palette = &view
	case "delete":
		if err := palettes.Delete(ctx, asked.ID); err != nil {
			reply.OK, reply.Error = false, err.Error()
			break
		}
		reply.ID = asked.ID
	default:
		reply.OK, reply.Error = false, "未知操作"
	}

	encoded, err := json.Marshal(reply)
	if err != nil {
		return "", errors.New("无法编码回答")
	}
	return string(encoded), nil
}

func viewOf(saved store.Palette) (paletteView, error) {
	swatches, err := color.Swatches(saved.Colors)
	if err != nil {
		return paletteView{}, errors.New("保存的配色里有无效颜色")
	}
	return paletteView{
		ID:        saved.ID,
		Name:      saved.Name,
		CreatedAt: saved.CreatedAt,
		Swatches:  swatches,
	}, nil
}

func deadline(milliseconds int64) (context.Context, context.CancelFunc) {
	if milliseconds <= 0 {
		return context.WithTimeout(context.Background(), 10*time.Second)
	}
	return context.WithDeadline(context.Background(), time.UnixMilli(milliseconds))
}

func write(client *hook.Client, reply response) {
	line, err := json.Marshal(reply)
	if err != nil {
		os.Exit(1)
	}
	if err := client.WriteLine(line); err != nil {
		os.Exit(1)
	}
}
