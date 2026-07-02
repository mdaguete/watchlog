{
  description = "WatchLog - Personal TV show and movie tracking app";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gnumake
            sqlite
            gopls
            gotools
            goreleaser
          ];

          shellHook = ''
            echo "WatchLog dev shell"
            echo "  make run               - Build and start server"
            echo "  make import            - Import TVTime data"
            echo "  goreleaser release     - Create release"
            echo ""
            echo "Set TMDB_API_KEY env var for TMDB integration"
          '';
        };
      });
}
