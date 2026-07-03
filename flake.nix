{
  description = "Spese — sheets-only net worth + expense tracker (ADR-0020)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        spesePackage = subPackages: mainProgram: pkgs.buildGoModule {
          pname = mainProgram;
          version = "0.2.0";
          src = ./.;

          # Recompute with `nix build` if go.mod changes.
          vendorHash = "sha256-SN4cccqmNqvKOUMTiH1KGkpgkBroRVEsGMOv36gYx+A=";

          ldflags = [ "-s" "-w" ];
          inherit subPackages;

          meta = with pkgs.lib; {
            description = "Personal net worth + expense tracker backed by Google Sheets";
            homepage = "https://github.com/emiliopalmerini/spese";
            license = licenses.mit;
            inherit mainProgram;
          };
        };
      in
      {
        packages = {
          default = spesePackage [ "cmd/spese" "cmd/spese-worker" ] "spese";
          import-sheets = spesePackage [ "cmd/spese-import-sheets" ] "spese-import-sheets";

          docker = pkgs.dockerTools.buildLayeredImage {
            name = "spese";
            tag = "latest";
            contents = [ self.packages.${system}.default ];
            config = {
              Cmd = [ "/bin/spese" ];
              ExposedPorts = { "8080/tcp" = { }; };
              Env = [ "SPESE_PORT=8080" ];
            };
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_25
            gopls
            golangci-lint
            air
          ];

          shellHook = ''
            echo "spese v2 development shell"
            echo ""
            echo "Available commands:"
            echo "  make run    — Start the server"
            echo "  make test   — Run tests"
            echo "  make build  — Build binary"
            echo "  air         — Hot reload"
          '';
        };
      }
    ) // {
      nixosModules.default = { config, lib, pkgs, ... }:
        with lib;
        let
          cfg = config.services.spese;
          serviceEnvironment = {
            SPESE_DB_PATH = cfg.dbPath;
            SPESE_RABBITMQ_QUEUE = cfg.rabbitMQQueue;
            GOOGLE_SPREADSHEET_ID = cfg.googleSpreadsheetId;
            GOOGLE_SERVICE_ACCOUNT_FILE = toString cfg.googleServiceAccountFile;
          } // optionalAttrs (cfg.rabbitMQUrl != "") {
            SPESE_RABBITMQ_URL = cfg.rabbitMQUrl;
          };
        in
        {
          options.services.spese = {
            enable = mkEnableOption "spese net worth tracker";

            package = mkOption {
              type = types.package;
              default = self.packages.${pkgs.system}.default;
              defaultText = literalExpression "spese.packages.\${pkgs.system}.default";
              description = "The spese package to use";
            };

            port = mkOption {
              type = types.port;
              default = 8080;
              description = "Port to listen on";
            };

            googleSpreadsheetId = mkOption {
              type = types.str;
              description = "Google Spreadsheet ID (v2 sheet).";
            };

            googleServiceAccountFile = mkOption {
              type = types.path;
              description = "Path to Google service account JSON file.";
            };

            dbPath = mkOption {
              type = types.str;
              default = "/var/lib/spese/spese.db";
              description = "Path to the local SQLite database.";
            };

            rabbitMQUrl = mkOption {
              type = types.str;
              default = "";
              description = "RabbitMQ or AMQPCloud URL. Prefer environmentFile for secrets.";
            };

            rabbitMQQueue = mkOption {
              type = types.str;
              default = "spese.sheet-sync";
              description = "RabbitMQ queue name used for sheet-sync messages.";
            };

            environmentFile = mkOption {
              type = types.nullOr types.path;
              default = null;
              description = "Path to environment file with secrets.";
            };
          };

          config = mkIf cfg.enable {
            systemd.services.spese = {
              description = "Spese net worth tracker";
              wantedBy = [ "multi-user.target" ];
              after = [ "network.target" ];

              serviceConfig = {
                Type = "simple";
                ExecStart = "${cfg.package}/bin/spese";
                Restart = "always";
                RestartSec = 5;

                # Hardening
                DynamicUser = true;
                ProtectSystem = "strict";
                ProtectHome = true;
                PrivateTmp = true;
                NoNewPrivileges = true;
                ProtectKernelTunables = true;
                ProtectKernelModules = true;
                ProtectControlGroups = true;
                RestrictNamespaces = true;
                RestrictRealtime = true;
                RestrictSUIDSGID = true;
                LockPersonality = true;
              } // optionalAttrs (cfg.environmentFile != null) {
                EnvironmentFile = cfg.environmentFile;
              };

              environment = {
                SPESE_PORT = toString cfg.port;
              } // serviceEnvironment;
            };

            systemd.services.spese-worker = {
              description = "Spese sheet sync worker";
              wantedBy = [ "multi-user.target" ];
              after = [ "network.target" "spese.service" ];

              serviceConfig = {
                Type = "simple";
                ExecStart = "${cfg.package}/bin/spese-worker";
                Restart = "always";
                RestartSec = 5;

                # Hardening
                DynamicUser = true;
                ProtectSystem = "strict";
                ProtectHome = true;
                PrivateTmp = true;
                NoNewPrivileges = true;
                ProtectKernelTunables = true;
                ProtectKernelModules = true;
                ProtectControlGroups = true;
                RestrictNamespaces = true;
                RestrictRealtime = true;
                RestrictSUIDSGID = true;
                LockPersonality = true;
              } // optionalAttrs (cfg.environmentFile != null) {
                EnvironmentFile = cfg.environmentFile;
              };

              environment = serviceEnvironment;
            };
          };
        };
    };
}
