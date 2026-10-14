{
  description = "NVUE schema generation and configuration validation";

  # 26.05 still supports Intel macOS as well as Apple Silicon and Linux.
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "aarch64-darwin"
        "x86_64-darwin"
        "aarch64-linux"
        "x86_64-linux"
      ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
          package = pkgs.callPackage ./nix/package.nix {
            version = "git-${self.shortRev or self.dirtyShortRev or "dirty"}";
          };
        in
        {
          default = package;
          nvueschema = package;
        }
      );

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/nvueschema";
          meta.description = self.packages.${system}.default.meta.description;
        };
      });

      checks = forAllSystems (system: {
        package = self.packages.${system}.default;
      });

      formatter = forAllSystems (system: nixpkgs.legacyPackages.${system}.nixfmt);
    };
}
