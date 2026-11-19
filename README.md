# Modeldesk

[![Build and test](https://github.com/sarisman84/modeldesk/actions/workflows/checks.yml/badge.svg?branch=main)](https://github.com/sarisman84/modeldesk/actions/workflows/checks.yml)
[![Go 1.23 or newer](https://img.shields.io/badge/Go-1.23%2B-007d9c)](go.mod)
[![MIT License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

## Introduction

Modeldesk is a local web app for managing GGUF model files and monitoring llama.cpp and llama-swap on Windows and NixOS. Import a file from disk or download one from Hugging Face, then reuse it across launch profiles. For example, you can compare two context lengths without downloading the weights twice.

## Setup

You can organize model files immediately. To run inference, install `llama-server` separately (and `llama-swap` for swap profiles), or connect to a server already running on the same machine.

### NixOS

With Nix flakes enabled, run:

```bash
nix run github:sarisman84/modeldesk
```

### Windows

With Git and Go 1.23 or newer installed, run in PowerShell:

```powershell
git clone https://github.com/sarisman84/modeldesk.git
cd modeldesk
go build -o modeldesk.exe .
.\modeldesk.exe
```

### First launch

Modeldesk opens the dashboard at <http://127.0.0.1:43120> by default. Choose **Runtime → Connect an existing server** to check a server’s status, or open **Model files** to add your first GGUF. See the [quickstart](docs/quickstart.md) for launch profiles, runtime configuration, and storage locations.

## License

Modeldesk is distributed under the [MIT License](LICENSE).
