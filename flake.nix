{
  description = "Klados — Kubernetes desktop client";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forEachSystem = nixpkgs.lib.genAttrs systems;
      pkgsFor = system: nixpkgs.legacyPackages.${system};
    in
    {
      packages = forEachSystem (system:
        let
          pkgs = pkgsFor system;
        in
        rec {
          klados = pkgs.callPackage
            ({ lib
             , buildGoModule
             , fetchPnpmDeps
             , pnpmConfigHook
             , pnpm_10
             , nodejs
             , pkg-config
             , gtk3
             , webkitgtk_4_1
             , wrapGAppsHook3
             , copyDesktopItems
             , makeDesktopItem
             }:

              buildGoModule (finalAttrs: {
                pname = "klados";
                version = "0.0.1";

                src = lib.cleanSource ./.;

                vendorHash = "sha256-FjIdfPuBsUDYwWkWIXuknnC9sh3I/UlXgYc5hTR7I3g=";

                # Wails' webview2 package has Windows-only `//go:embed`
                # directives that trip `go mod download`'s embed resolver on
                # Linux. proxyVendor caches modules as zips so embeds are
                # never resolved during vendoring.
                proxyVendor = true;

                # The `goModules` sub-derivation only needs Go — strip
                # pnpm/node/desktop hooks (they require `pnpmDeps`, which
                # isn't relevant when just fetching Go dependencies).
                overrideModAttrs = old: {
                  nativeBuildInputs = builtins.filter
                    (x: !(builtins.elem x [
                      pnpm_10
                      pnpmConfigHook
                      nodejs
                      wrapGAppsHook3
                      copyDesktopItems
                    ]))
                    old.nativeBuildInputs;
                  preBuild = "";
                };

                # Pin pnpm to a specific version so the pnpmDeps hash is
                # decoupled from the consumer's nixpkgs (different pnpm
                # versions produce different `pnpm fetch` outputs).
                pnpmDeps = fetchPnpmDeps {
                  inherit (finalAttrs) pname version src;
                  hash = "sha256-duoTavVxrB8evtOiq0ell+qwL1JPaimdicb4xmsG4L4=";
                  fetcherVersion = 3;
                  pnpm = pnpm_10;
                };

                nativeBuildInputs = [
                  pkg-config
                  nodejs
                  pnpm_10
                  pnpmConfigHook
                  wrapGAppsHook3
                  copyDesktopItems
                ];

                buildInputs = [
                  gtk3
                  webkitgtk_4_1
                ];

                subPackages = [ "." ];
                tags = [ "production" ];
                ldflags = [ "-s" "-w" ];

                # Go's `//go:embed all:frontend/dist` requires the frontend to
                # be built before `go build` runs.
                preBuild = ''
                  (cd frontend && pnpm run build)
                '';

                desktopItems = [
                  (makeDesktopItem {
                    name = "klados";
                    desktopName = "Klados";
                    exec = "klados";
                    icon = "klados";
                    categories = [ "Development" ];
                    keywords = [ "kubernetes" "k8s" "wails" ];
                    terminal = false;
                  })
                ];

                postInstall = ''
                  install -Dm644 build/appicon.png \
                    $out/share/icons/hicolor/512x512/apps/klados.png
                '';

                meta = {
                  description = "Kubernetes desktop client";
                  homepage = "https://github.com/Vilsol/klados";
                  license = lib.licenses.mit;
                  mainProgram = "klados";
                  platforms = lib.platforms.linux;
                };
              }))
            { };

          default = klados;
        });

      apps = forEachSystem (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/klados";
        };
      });

      devShells = forEachSystem (system:
        let pkgs = pkgsFor system; in
        {
          default = pkgs.mkShell {
            buildInputs = with pkgs; [
              pkg-config
              gtk4
              webkitgtk_4_1
              webkitgtk_6_0
            ];
          };
        });
    };
}
