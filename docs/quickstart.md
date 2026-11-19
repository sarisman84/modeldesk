# Run Modeldesk

Modeldesk opens a browser dashboard on `http://127.0.0.1:43120`. Install `llama-server` or `llama-swap` separately. You can also use Modeldesk to manage model files before either runtime is installed.

- **Windows x64:** run `dist/modeldesk-windows-amd64.exe`. Keep its console open while using the app.
- **NixOS:** run `nix run .` from the project directory with flakes enabled.
- **From source:** use Go 1.23 or newer and run `go run .`.

## First use

1. If your server already runs, choose **Runtime → Connect an existing server**. Supply its loopback HTTP address. Modeldesk refreshes status every five seconds while the dashboard is visible. Metrics require a runtime that exposes them; use `--metrics` on an external `llama-server`.
2. In **Model files**, paste an absolute path to import an existing GGUF, or enter a public Hugging Face repository and pick a GGUF by name and size. Downloads check the repository revision, file size, and SHA-256 when published. Keep the tab open until the transfer finishes. Choose a single-file model; the downloader does not assemble split GGUFs.
3. In **Profiles**, name a configuration, select the file and runtime, and choose an unused port. If the executables are outside `PATH`, enter their full paths. Extra arguments take one argument per line, without surrounding shell quotes.
4. Choose **Launch**. A llama.cpp profile loads its weights; a llama-swap profile starts a proxy that loads them on the first inference request. Point your inference client at `http://127.0.0.1:<profile-port>/v1` using the profile name as the model ID.

## Existing configurations

To edit an existing runtime, supply its YAML path (llama-swap) or preset INI path (llama.cpp) when connecting. Edit the native text, preview the changes, then choose **Back up and apply**. Modeldesk saves a sibling `.bak` copy and rejects edits if the file changed after it was opened. The editor does not validate runtime-specific syntax. A watched llama-swap config may reload immediately; otherwise restart the runtime using its normal launcher. On NixOS, edit the declarative source of any read-only Nix store config instead.

## Data and removal

By default, the library, downloads and profile logs live under `%AppData%\modeldesk` on Windows, or `${XDG_CONFIG_HOME:-$HOME/.config}/modeldesk` on NixOS. Use `-data <folder>` to choose another location, `-port <number>` to change the dashboard port, or `-open=false` to skip opening a browser. Use `Ctrl+C` in the launcher to stop Modeldesk and its managed processes.

**Forget** removes a library entry. **Delete from disk** also removes the file at the displayed path, even if you imported it. Remove referencing Modeldesk profiles first. External applications may still use the same path.
