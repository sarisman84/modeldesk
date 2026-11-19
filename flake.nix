{
  description = "Local llama.cpp and llama-swap model manager";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  outputs = { self, nixpkgs }: {
    packages = nixpkgs.lib.genAttrs [ "x86_64-linux" "aarch64-linux" ] (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        default = pkgs.buildGoModule {
          pname = "modeldesk";
          version = "0.1.0";
          src = pkgs.lib.fileset.toSource {
            root = ./.;
            fileset = pkgs.lib.fileset.unions [
              ./go.mod
              ./web
              (pkgs.lib.fileset.fileFilter (f: f.hasExt "go") ./.)
            ];
          };
          vendorHash = null;
          meta = {
            description = "Local browser dashboard for GGUF libraries and inference servers";
            mainProgram = "modeldesk";
            platforms = pkgs.lib.platforms.linux;
          };
        };
      }
    );
  };
}
