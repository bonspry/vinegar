// Package config implements types and routines to configure Vinegar.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/apprehensions/rbxbin"
	"github.com/apprehensions/wine"
	"github.com/vinegarhq/vinegar/internal/dirs"
	"github.com/vinegarhq/vinegar/sysinfo"
)

// Studio is a representation of the deployment and behavior
// of Roblox Studio.
type Studio struct {
	GameMode      bool          `toml:"gamemode"`
	WineRoot      string        `toml:"wineroot"`
	Launcher      string        `toml:"launcher"`
	Quiet         bool          `toml:"quiet"`
	DiscordRPC    bool          `toml:"discord_rpc"`
	ForcedVersion string        `toml:"forced_version"`
	Channel       string        `toml:"channel"`
	Dxvk          bool          `toml:"dxvk"`
	DxvkVersion   string        `toml:"dxvk_version"`
	ForcedGpu     string        `toml:"gpu"`
	Renderer      string        `toml:"renderer"`
	Env           Environment   `toml:"env"`
	FFlags        rbxbin.FFlags `toml:"fflags"`
}

// Config is a representation of the Vinegar configuration.
type Config struct {
	SanitizeEnv bool        `toml:"sanitize_env"`
	Studio      Studio      `toml:"studio"`
	Env         Environment `toml:"env"`
}

var (
	ErrNeedDXVKRenderer = errors.New("dxvk is only valid with d3d renderers")
	ErrWineRootAbs      = errors.New("wine root path is not an absolute path")
	ErrWineRootInvalid  = errors.New("no wine binary present in wine root")
)

// Load will load the configuration file; if it doesn't exist, it
// will fallback to the default configuration.
func Load() (*Config, error) {
	d := Default()

	if _, err := os.Stat(dirs.ConfigPath); errors.Is(err, os.ErrNotExist) {
		return d, nil
	}

	if err := Decode(d); err != nil {
		return nil, err
	}

	return d, nil
}

// Decode will load the configuration file into the given Config.
func Decode(cfg *Config) error {
	if _, err := toml.DecodeFile(dirs.ConfigPath, &cfg); err != nil {
		return err
	}

	return cfg.Setup()
}

// Default returns a default configuration.
func Default() *Config {
	return &Config{
		Env: Environment{
			"WINEARCH":                    "win64",
			"WINEDEBUG":                   "fixme-all,err-kerberos,err-ntlm",
			"WINEESYNC":                   "1",
			"WINEDLLOVERRIDES":            "dxdiagn,winemenubuilder.exe,mscoree,mshtml=",
			"DXVK_LOG_LEVEL":              "warn",
			"DXVK_LOG_PATH":               "none",
			"MESA_GL_VERSION_OVERRIDE":    "4.4",
			"__GL_THREADED_OPTIMIZATIONS": "1",
		},

		Studio: Studio{
			Dxvk:        true,
			DxvkVersion: "2.5.3",
			GameMode:    true,
			ForcedGpu:   "prime-discrete",
			Renderer:    "D3D11",
			Channel:     "LIVE",
			DiscordRPC:  true,
			FFlags:      make(rbxbin.FFlags),
			Env:         make(Environment),
		},
	}
}

func (s *Studio) LauncherPath() (string, error) {
	return exec.LookPath(strings.Fields(s.Launcher)[0])
}

func (c *Config) Setup() error {
	if c.SanitizeEnv {
		SanitizeEnv()
	}

	c.Env.Setenv()

	if err := c.Studio.setup(); err != nil {
		return fmt.Errorf("studio: %w", err)
	}

	return nil
}

func (s *Studio) setup() error {
	if !strings.HasPrefix(s.Renderer, "D3D11") && s.Dxvk {
		return ErrNeedDXVKRenderer
	}

	if s.Launcher != "" {
		if _, err := s.LauncherPath(); err != nil {
			return fmt.Errorf("bad launcher: %w", err)
		}
	}

	if s.WineRoot != "" {
		pfx := wine.New("", s.WineRoot)
		w := pfx.Wine("")
		if w.Err != nil {
			return fmt.Errorf("wineroot: %w", w.Err)
		}
		if pfx.IsProton() && w.Args[0] != "umu-run" && !sysinfo.InFlatpak {
			slog.Warn("wineroot: umu-run recommended for Proton usage!")
		}
	}

	if err := s.FFlags.SetRenderer(s.Renderer); err != nil {
		return err
	}

	if s.Channel == "LIVE" || s.Channel == "live" {
		s.Channel = ""
	}

	return s.pickCard()
}
