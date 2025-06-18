//go:build mage

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/autonomouskoi/akcore/modules"
	"github.com/autonomouskoi/mageutil"
)

var (
	baseDir   string
	outDir    string
	pluginDir string
	version   string
	webSrcDir string
	webOutDir string

	Default = Plugin
)

func init() {
	// set up our paths
	var err error
	baseDir, err = os.Getwd()
	if err != nil {
		panic(err)
	}
	outDir = filepath.Join(baseDir, "out")
	pluginDir = filepath.Join(outDir, "plugin")
	webSrcDir = filepath.Join(baseDir, "web")
	webOutDir = filepath.Join(webSrcDir, "out")
}

// clean up intermediate products
func Clean() error {
	for _, dir := range []string{
		outDir, webOutDir,
	} {
		if err := sh.Rm(dir); err != nil {
			return fmt.Errorf("deleting %s: %w", dir, err)
		}
	}
	return nil
}

// What's needed for running the plugin as a dev plugin
func Plugin() {
	mg.Deps(
		Icon,
		Manifest,
		WASM,
		Web,
	)
}

// Load plugin version from VERSION file
func Version() error {
	b, err := os.ReadFile(filepath.Join(baseDir, "VERSION"))
	if err != nil {
		return fmt.Errorf("reading VERSION: %w", err)
	}
	version = strings.TrimSpace(string(b))
	return nil
}

// Build the plugin for release
func Release() error {
	mg.Deps(Plugin, Version)
	filename := fmt.Sprintf("trackstar-twitchchat-%s.akplugin", version)
	return mageutil.ZipDir(pluginDir, filepath.Join(outDir, filename))
}

// Generate tinygo code for our protos
func GoProtos() error {
	protos, err := mageutil.DirGlob(baseDir, "*.proto")
	if err != nil {
		return fmt.Errorf("globbing %s: %w", baseDir)
	}

	for _, protoFile := range protos {
		srcPath := filepath.Join(baseDir, protoFile)
		dstPath := filepath.Join(baseDir, "go", strings.TrimSuffix(protoFile, ".proto")+".pb.go")
		err := mageutil.TinyGoProto(dstPath, srcPath, baseDir)
		if err != nil {
			return fmt.Errorf("generating from %s: %w", srcPath, err)
		}
	}
	return nil
}

// Create our output dir
func OutDir() error {
	return mageutil.Mkdir(outDir)
}

// Create our plugin dir
func PluginDir() error {
	mg.Deps(OutDir)
	return mageutil.Mkdir(pluginDir)
}

// Compile our WASM code
func WASM() error {
	mg.Deps(PluginDir, GoProtos)

	srcDir := filepath.Join(baseDir, "go", "main")
	outFile := filepath.Join(pluginDir, "twitchchat.wasm")
	return mageutil.TinyGoWASM(srcDir, outFile)
}

// Copy our icon
func Icon() error {
	mg.Deps(PluginDir)
	iconPath := filepath.Join(baseDir, "icon.svg")
	outPath := filepath.Join(pluginDir, "icon.svg")
	return mageutil.CopyFiles(map[string]string{
		iconPath: outPath,
	})
}

// Write our manifest
func Manifest() error {
	mg.Deps(PluginDir, Version)
	manifestPB := &modules.Manifest{
		Name:        "trackstar-twitchchat",
		Title:       "TS: Twitch Chat",
		Id:          "62071945ac98ada1",
		Description: "Trackstar integration with Twitch chat",
		Version:     version,
		WebPaths: []*modules.ManifestWebPath{
			{
				Path:        "https://autonomouskoi.org/plugin-trackstar-twitchchat.html",
				Type:        modules.ManifestWebPathType_MANIFEST_WEB_PATH_TYPE_HELP,
				Description: "Help!",
			},
			{
				Path:        "/m/trackstar-twitchchat/embed_ctrl.js",
				Type:        modules.ManifestWebPathType_MANIFEST_WEB_PATH_TYPE_EMBED_CONTROL,
				Description: "Configuration",
			},
			{
				Path:        "/m/trackstar-twitchchat/index.html",
				Type:        modules.ManifestWebPathType_MANIFEST_WEB_PATH_TYPE_CONTROL_PAGE,
				Description: "Configuration",
			},
		},
		CustomWebDir: true,
	}
	manifest, err := protojson.Marshal(manifestPB)
	if err != nil {
		return fmt.Errorf("marshalling proto: %w", err)
	}
	buf := &bytes.Buffer{}
	if err := json.Indent(buf, manifest, "", "  "); err != nil {
		return fmt.Errorf("formatting manifest JSON: %w", err)
	}
	fmt.Fprintln(buf)
	manifestPath := filepath.Join(pluginDir, "manifest.json")
	_, err = os.Stat(manifestPath)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(manifestPath, buf.Bytes(), 0644)
}

// install NPM modules for web content
func NPMModules() error {
	nmPath := filepath.Join(webSrcDir, "node_modules")
	if _, err := os.Stat(nmPath); err == nil {
		return nil
	}
	if err := os.Chdir(webSrcDir); err != nil {
		return fmt.Errorf("switching to %s: %w", webSrcDir, err)
	}
	if err := sh.Run("npm", "install"); err != nil {
		return fmt.Errorf("running npm install: %w", err)
	}
	return nil
}

// Create our web output dir
func WebOutDir() error {
	return mageutil.Mkdir(webOutDir)
}

// Generate our TypeScript protos
func TSProtos() error {
	mg.Deps(WebOutDir, NPMModules)
	if err := os.Chdir(webSrcDir); err != nil {
		return fmt.Errorf("switching to %s: %w", webSrcDir, err)
	}
	return mageutil.TSProtosInDir(webOutDir, baseDir, filepath.Join(webSrcDir, "node_modules"))
}

// Compile our TS code
func TS() error {
	mg.Deps(TSProtos)
	return mageutil.BuildTypeScript(webSrcDir, webSrcDir, webOutDir)
}

// Copy static web content
func WebSrcCopy() error {
	mg.Deps(WebOutDir)
	filenames := []string{"index.html"}
	if err := mageutil.CopyInDir(webOutDir, webSrcDir, filenames...); err != nil {
		return fmt.Errorf("copying: %w", err)
	}
	return nil
}

// All our web targets
func Web() error {
	mg.Deps(
		WebSrcCopy,
		TS,
	)
	return mageutil.SyncDirBasic(webOutDir, filepath.Join(pluginDir, "web"))
}
