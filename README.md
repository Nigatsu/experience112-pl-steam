# experience112-pl-steam

Installs the **Polish localisation** of *eXperience112* (Lexis Numérique,
2007) — voices, subtitles, in-game documents, mail, lip-sync — into the
**Steam release** of the game, using the player's own copy of the Polish
retail disc (Techland).

The tool contains **no game data**. Everything is read from the user's own
copies: the Polish disc / ISO image and the Steam installation.

Polish user guide: [README.pl.md](README.pl.md).

## Why a tool is needed

The Polish retail build and the Steam build run different builds of the same
engine:

| | Polish retail | Steam (EN/ES/FR/DE) |
|---|---|---|
| Engine | UNICODE build | 8-bit (ANSI) build |
| Text resources | UTF-16 | Windows-1252/1250 |
| Archive table layout | wide (UTF-16 paths) | ANSI |
| Fonts | rasterised per `.cci` charset | fixed byte codes `0x20`–`0xFE` |

Simply copying the Polish archive into the Steam game does not work. The
patcher therefore:

1. extracts `loc_plPL.arc` from the InstallShield cabinets on the disc or
   ISO (no mounting needed), verifying the installer's MD5;
2. rebuilds it in the Steam archive layout, following the file list of
   Steam's `loc_enUS.arc`;
3. converts every text resource (`.xml`, `.loc`, `.lua`, `.lps` lip-sync…)
   from UTF-16 to Windows-1250, keeping Steam's XML headers and CRLF line
   endings;
4. remaps the byte codes Polish uses in the game fonts to the Polish glyphs;
5. keeps English versions of the few files the Polish release lacks;
6. installs the archive, switches the game language to `plPL` and backs up
   everything it replaces (`PL_patch_backup`);
7. fixes clipped descenders (g, j, y): the engine applies a half-texel font
   correction only on NVIDIA GPUs. Under Wine/CrossOver the reported GPU
   vendor is changed in the bottle registry (tested). On native Windows with
   another GPU a 3-byte patch of `engine.dll` is offered (untested).

`-restore` undoes all of it.

### Steam language editions

| Edition | Layout | Patcher |
|---|---|---|
| English | English base + `loc_enUS.arc` | supported (target) |
| Spanish | English base + loose override files | supported (overrides moved to backup) |
| French | French base, texts inside `gamedata.arc` | asks to switch Steam to English |
| German | French base + `loc_deDE.arc` | asks to switch Steam to English |

## Usage

Put the Polish disc image (`.iso`) next to the program, or insert the disc,
then run `exp112-pl-patch.exe`. Under CrossOver, run it in the bottle that
contains Steam.

```
exp112-pl-patch [-game DIR] [-source PATH] [-restore] [-nogfx] [-yes]

  -game     Steam game folder (…\steamapps\common\Experience112); auto-detected
  -source   Polish .iso, disc folder (with data1.hdr) or installed Polish game;
            auto-detected (ISO next to the program, drives, default install path)
  -restore  restore the original game files
  -nogfx    skip the clipped-descender fix
  -yes      non-interactive: accept all defaults
```

## Building

Requires Go 1.22+.

```sh
make test     # go vet + unit tests
make build    # dist/exp112-pl-patch.exe (Windows, 386)
```

The code builds and its tests run on any OS; Windows-only parts (registry,
drive letters, GPU detection) live in `internal/platform` with stubs
elsewhere.

## Layout

| Package | Purpose |
|---|---|
| `cmd/exp112-pl-patch` | command line and console UI (Polish) |
| `internal/patch` | finding game and source, building, installing, restoring |
| `internal/arc` | Lexis engine `ARC0` archives and their per-file keys |
| `internal/installshield` | InstallShield v6+ cabinet extraction |
| `internal/iso9660` | read-only ISO 9660 image access |
| `internal/textconv` | UTF-16 → Windows-1250 conversion of game text resources |
| `internal/ttf` | TrueType `cmap` remapping |
| `internal/platform` | Windows registry, drives, GPU and Wine detection |

### ARC0 format notes

```
header : "ARC0" u32 version(=1) u32 count u32 tocOffset
data   : each file XOR'd dword-wise with its own key (tail <4 bytes plain)
toc    : name[N] path[P] u32 offset u32 size u32 fileKey, XOR'd with the archive key
trailer: u32 archive key = XOR of all file keys
key    : MD5 whose rotate uses a signed right shift, state words folded with XOR
```

`N`/`P` are `0x100`/`0x200` (UTF-16) in UNICODE builds and `0x80`/`0x100`
(8-bit) in ANSI builds. The engine verifies the key of every file it reads.

## Disclaimer

Not affiliated with or endorsed by Lexis Numérique, Micro Application,
Techland, Anuman/Microïds or Valve. *eXperience112* is a trademark of its
respective owners. You need legitimate copies of both the Polish retail
release and the Steam release; this project does not distribute any part of
the game.

## License

[MIT](LICENSE)
