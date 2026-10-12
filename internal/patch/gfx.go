package patch

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Nigatsu/experience112-pl-steam/internal/platform"
)

// The engine applies a half-texel UV correction to font glyphs only when the
// Direct3D adapter vendor is NVIDIA (0x10de). On other GPUs the bottom row of
// every glyph cell is lost, cutting off descenders (g, j, y).
//
// Under Wine the reported vendor can be changed in the registry. On Windows
// the check in engine.dll (Steam build) is patched instead:
//
//	cmp eax, 10DEh ; mov eax, [ebp-0ACh] ; sete bl   ->   ... ; mov bl, 1 ; nop
const (
	nvidiaVendorID  = 0x10de
	wineVendorValue = "VideoPciVendorID"
	stateFile       = "state.txt"
	stateWineVendor = "wine_vendor_prev"
	engineCheckOff  = 0x31359
)

var (
	engineOriginal = []byte{0x3d, 0xde, 0x10, 0x00, 0x00, 0x8b, 0x85, 0x54, 0xff, 0xff, 0xff, 0x0f, 0x94, 0xc3}
	enginePatched  = []byte{0x3d, 0xde, 0x10, 0x00, 0x00, 0x8b, 0x85, 0x54, 0xff, 0xff, 0xff, 0xb3, 0x01, 0x90}
)

var errUnknownEngine = errors.New("nieznana wersja engine.dll — pominięto")

// patchEngine applies the font fix to an engine.dll image in place.
// It reports whether b was already patched.
func patchEngine(b []byte) (alreadyPatched bool, err error) {
	if len(b) < engineCheckOff+len(engineOriginal) {
		return false, errUnknownEngine
	}
	cur := b[engineCheckOff : engineCheckOff+len(engineOriginal)]
	switch {
	case bytes.Equal(cur, enginePatched):
		return true, nil
	case bytes.Equal(cur, engineOriginal):
		copy(cur, enginePatched)
		return false, nil
	}
	return false, errUnknownEngine
}

// applyTextFix applies the descender-clipping fix appropriate for the system.
func applyTextFix(game string, ui UI) error {
	bak := filepath.Join(game, BackupDirName)
	if platform.IsWine() {
		ui.Printf("      Wykryto Wine/CrossOver: zgłoszenie karty NVIDIA dla Direct3D (rejestr butelki).\n")
		if !ui.Confirm("      Zastosować?", true) {
			return nil
		}
		state := readState(bak)
		if _, ok := state[stateWineVendor]; !ok {
			if v, ok := platform.UserDword(platform.WineD3DKey, wineVendorValue); ok {
				state[stateWineVendor] = strconv.FormatUint(uint64(v), 10)
			} else {
				state[stateWineVendor] = "none"
			}
		}
		if err := platform.SetUserDword(platform.WineD3DKey, wineVendorValue, nvidiaVendorID); err != nil {
			return err
		}
		ui.Printf("      HKCU\\%s\\%s = 0x10de\n", platform.WineD3DKey, wineVendorValue)
		return writeState(bak, state)
	}

	gpu, known := platform.PrimaryGPU()
	if known {
		ui.Printf("      Karta graficzna: %s\n", gpu.Name)
		if gpu.NVIDIA {
			ui.Printf("      Karta NVIDIA — poprawka niepotrzebna.\n")
			return nil
		}
	}
	ui.Printf("      Poprawka engine.dll: korekta pół-teksela czcionek także dla kart innych niż NVIDIA.\n")
	if !ui.Confirm("      Zastosować? (kopia engine.dll zostanie zachowana)", true) {
		return nil
	}
	eng := filepath.Join(game, "engine.dll")
	b, err := os.ReadFile(eng)
	if err != nil {
		return err
	}
	already, err := patchEngine(b)
	if err != nil {
		return err
	}
	if already {
		ui.Printf("      engine.dll już poprawiony.\n")
		return nil
	}
	if backup := filepath.Join(bak, "engine.dll"); !exists(backup) {
		if err := copyFile(eng, backup); err != nil {
			return err
		}
	}
	if err := os.WriteFile(eng, b, 0o644); err != nil {
		return err
	}
	ui.Printf("      engine.dll poprawiony.\n")
	return nil
}

// revertTextFix restores the Wine registry value recorded in the backup.
func revertTextFix(bak string) {
	prev, ok := readState(bak)[stateWineVendor]
	if !ok || !platform.IsWine() {
		return
	}
	if prev == "none" {
		platform.DeleteUserValue(platform.WineD3DKey, wineVendorValue)
	} else if v, err := strconv.ParseUint(prev, 10, 32); err == nil {
		platform.SetUserDword(platform.WineD3DKey, wineVendorValue, uint32(v))
	}
}

func readState(bak string) map[string]string {
	m := map[string]string{}
	b, _ := os.ReadFile(filepath.Join(bak, stateFile))
	for _, line := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			m[k] = v
		}
	}
	return m
}

func writeState(bak string, m map[string]string) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k + "=" + m[k] + "\n")
	}
	if err := os.MkdirAll(bak, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(bak, stateFile), []byte(sb.String()), 0o644)
}
