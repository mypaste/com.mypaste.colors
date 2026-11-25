# 配色

MyPaste extension `com.mypaste.colors` (version `1.2.0`).

设置窗口的一级菜单「配色」，清单声明系统级快捷键 `Ctrl+Alt+C`，在任何应用里都能一步打开这一页（设置 › 快捷键 › 扩展菜单 可以改或清空，被占用的和弦由宿主跳过并记日志）。打开时落在取色页，并先用上次复制或选取的颜色铺好色阶——这份「当前颜色」记在扩展私有目录的 `current.json`，还没有用过任何颜色时退回今天的主色。每日一页按本机日期给出五款颜色——主色、浅调、深调、强调与中性，同一天永远得到同一套配色；日历列出这个月每一天的配色，点一天就翻到那天，色块区只保留一屏内的高度；取色一页把任意颜色（系统取色器、HEX、RGB 或 HSL）展开成 5 / 7 / 9 / 11 阶深浅色阶，所选颜色固定在正中并标出；收藏一页把每日配色和色阶命名保存下来，随时翻看、复制或删除。所有颜色计算在扩展自己的进程里完成，收藏只存在扩展的私有目录，卸载即随之删除；复制交给宿主的剪贴板通道，页面不联网、不读写存储。

The page is one settings-window menu (`colors.panel`) and holds no colour rule: it asks the runtime and lays out what comes back. The menu declares `Ctrl+Alt+C` as its system-wide shortcut, so the page is one chord away from any app; Settings › Shortcuts › Extension Menus can change or clear it, and the host skips a chord that is already taken. The panel opens on the picker tab with the colour it last handed over — copied or picked here — already drawn as the scale's middle step; that current colour lives in the package's private `current.json`, and only a page that has never produced one falls back to the day's first swatch. The daily tab shows a date's five colours with their roles, names and HEX / RGB / HSL readings; the strip keeps one readable height and a month grid below shows every day's palette, so any day is one click away. The arrows still walk day by day, and the tab rolls over at midnight. The picker tab pins the picked colour at the middle step, labelled 所选, so the reader can see where their colour sits, and copies the scale as HEX lines, RGB values, HSL triplets or CSS custom properties. The saved tab keeps up to 200 named palettes (2–12 distinct colours, 40-character names) with a two-step delete.

Every rule lives in the package's own Go runtime: `colors.request` answers `daily` (a date's palette, seeded from the date so it cannot drift), `month` (every day of a month, so the grid and the strip cannot disagree), `ramp` (the scale around one colour), `current` / `remember` (the colour the page reopens on), `list`, `save` and `delete`. Colour conversion, Chinese colour naming and the contrast ink each swatch carries are in `internal/color`; near-white and near-black picks nudge later steps outward so no two steps of a scale repeat. The saved document lives in `internal/store` over `storage.plugin` — the only capability the manifest declares. The page never sees the document, the filesystem or the network.

This repository is the extension's own source home. It contains only this extension's tracked source files. The published `.mypaste-plugin` archives and the marketplace index live in [`mypaste-extensions-catalog`](https://github.com/mypaste/mypaste-extensions-catalog).

The manifest speaks MyPaste plugin protocol v3. Installation, capability mediation, and content-digest verification are performed by the MyPaste desktop host.

## Files

- `mypaste.plugin.json`: package identity, the menu entry, and the one capability (`storage.plugin`).
- `panel.html`: the menu page — three tabs, the host's theme tokens, and copy requests routed through the host.
- `src/`: the extension's own Go process — the daily palette, the tint/shade scale, colour names and inks, and the saved document.

## Build

```bash
cd src && CGO_ENABLED=0 go build -trimpath ./cmd/mypaste-colors
```

The released archive carries the `colors` binary beside the manifest (darwin arm64 and amd64) plus the page.

## License

MIT. See [LICENSE](LICENSE).
