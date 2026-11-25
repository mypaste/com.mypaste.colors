# 配色

MyPaste extension `com.mypaste.colors` (version `1.0.1`).

设置窗口的一级菜单「配色」。每日一页按本机日期给出五款颜色——主色、浅调、深调、强调与中性，同一天永远得到同一套配色，可以往前翻看这个月的每一天；取色一页把任意颜色（系统取色器、HEX、RGB 或 HSL）展开成 5 / 7 / 9 / 11 阶深浅色阶，所选颜色固定在正中并标出；收藏一页把每日配色和色阶命名保存下来，随时翻看、复制或删除。所有颜色计算在扩展自己的进程里完成，收藏只存在扩展的私有目录，卸载即随之删除；复制交给宿主的剪贴板通道，页面不联网、不读写存储。

The page is one settings-window menu (`colors.panel`) and holds no colour rule: it asks the runtime and lays out what comes back. The daily tab shows a date's five colours with their roles, names and HEX / RGB / HSL readings, walks backwards and forwards with the arrows, and rolls over at midnight. The picker tab pins the picked colour at the middle step, labelled 所选, so the reader can see where their colour sits, and copies the scale as HEX lines, RGB values, HSL triplets or CSS custom properties. The saved tab keeps up to 200 named palettes (2–12 distinct colours, 40-character names) with a two-step delete.

Every rule lives in the package's own Go runtime: `colors.request` answers `daily` (a date's palette, seeded from the date so it cannot drift), `ramp` (the scale around one colour), `list`, `save` and `delete`. Colour conversion, Chinese colour naming and the contrast ink each swatch carries are in `internal/color`; near-white and near-black picks nudge later steps outward so no two steps of a scale repeat. The saved document lives in `internal/store` over `storage.plugin` — the only capability the manifest declares. The page never sees the document, the filesystem or the network.

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
