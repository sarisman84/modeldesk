# modeldesk

Modeldesk lets you manage local GGUF files and see which models llama.cpp or llama-swap is serving. A model file can have several launch profiles, so trying a new context length doesn't require another download.

On NixOS with flakes enabled, run `nix run github:sarisman84/modeldesk`. On Windows, clone this repository and run `go build -o modeldesk.exe .` with Go 1.23 or newer, then start `modeldesk.exe`. The dashboard opens in your browser. Install the inference runtimes separately, or connect a server that is already running on the same machine.

See the [quickstart](docs/quickstart.md) for Hugging Face downloads, profile settings, and managing existing server configurations.
