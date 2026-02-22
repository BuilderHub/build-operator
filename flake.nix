{
  description = "BuilderHub BuildKit Operator - BuildKit as a Service on bare-metal Kubernetes";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachSystem [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ] (system:
      let
        pkgs = import nixpkgs { inherit system; };
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            kubernetes-controller-tools
            kustomize
            kubectl
            kind
            docker
            docker-buildx
            gnumake
          ];

          shellHook = ''
            echo "🔧 BuilderHub BuildKit Operator Dev Environment"
            echo "Go version: $(go version)"
            echo ""
            echo "Run 'go mod tidy' to fetch dependencies"
            echo "Run 'make generate' to generate CRDs"
            echo "Run 'make run' to start the operator"
            echo "Run 'make kind-create' to create a kind cluster"
            echo "Run 'make help' for available targets"
          '';
        };
      }
    );
}
