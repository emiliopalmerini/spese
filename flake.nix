{
  description = "Spese v2 — local SQLite ledger with an embedded React SPA";

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
          vendorHash = "sha256-7zHyanDJ/AP6i2FrNNWuvCO7+FHRwoZEwu3Qpol1Rco=";

          ldflags = [ "-s" "-w" ];
          inherit subPackages;

          meta = with pkgs.lib; {
            description = "Single-user personal finance ledger with a derived Google Sheets mirror";
            homepage = "https://github.com/emiliopalmerini/spese";
            license = licenses.mit;
            inherit mainProgram;
          };
        };
      in
      {
        packages = {
          default = spesePackage [ "cmd/spese" "cmd/spese-worker" "cmd/spese-migrate" ] "spese";
          migrate = spesePackage [ "cmd/spese-migrate" ] "spese-migrate";

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
            nodejs_24
            gopls
            golangci-lint
            air
          ];

          shellHook = ''
            echo "spese v2 development shell"
            echo ""
            echo "Available commands:"
            echo "  make run             — Start the server"
            echo "  make test            — Run Go and frontend tests"
            echo "  make frontend-build  — Rebuild embedded SPA"
            echo "  make build           — Build runtime and tools"
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
            users.groups.spese = { };
            users.users.spese = {
              isSystemUser = true;
              group = "spese";
            };

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
                User = "spese";
                Group = "spese";
                StateDirectory = "spese";
                UMask = "0077";
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
                User = "spese";
                Group = "spese";
                StateDirectory = "spese";
                UMask = "0077";
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
