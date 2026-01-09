{
  description = "Spese - Personal expense tracker with Google Sheets sync";

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
        packages = {
          default = pkgs.buildGoModule {
            pname = "spese";
            version = "0.1.0";
            src = ./.;

            vendorHash = "sha256-r1/vu45J3m9YRz6jQifiXNTSPMV1bd3JiAEn8uvP2VI=";

            # Build flags matching Makefile
            ldflags = [ "-s" "-w" ];

            # Only build the main binary
            subPackages = [ "cmd/spese" ];

            meta = with pkgs.lib; {
              description = "Personal expense tracker with Google Sheets sync";
              homepage = "https://github.com/emiliopalmerini/spese";
              license = licenses.mit;
              mainProgram = "spese";
            };
          };

          docker = pkgs.dockerTools.buildLayeredImage {
            name = "spese";
            tag = "latest";
            contents = [ self.packages.${system}.default ];
            config = {
              Cmd = [ "/bin/spese" ];
              ExposedPorts = { "8081/tcp" = { }; };
              Env = [
                "PORT=8081"
                "SQLITE_DB_PATH=/data/spese.db"
              ];
              Volumes = { "/data" = { }; };
            };
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_24
            gopls
            golangci-lint
            sqlc
            sqlite
            air
          ];

          shellHook = ''
            echo "spese development shell"
            echo ""
            echo "Available commands:"
            echo "  make run    - Start the server"
            echo "  make test   - Run tests"
            echo "  make build  - Build binary"
            echo "  air         - Hot reload development"
          '';
        };
      }
    ) // {
      nixosModules.default = { config, lib, pkgs, ... }:
        with lib;
        let
          cfg = config.services.spese;
        in
        {
          options.services.spese = {
            enable = mkEnableOption "spese expense tracker";

            package = mkOption {
              type = types.package;
              default = self.packages.${pkgs.system}.default;
              defaultText = literalExpression "spese.packages.\${pkgs.system}.default";
              description = "The spese package to use";
            };

            port = mkOption {
              type = types.port;
              default = 8081;
              description = "Port to listen on";
            };

            dataDir = mkOption {
              type = types.path;
              default = "/var/lib/spese";
              description = "Directory for SQLite database";
            };

            googleSpreadsheetId = mkOption {
              type = types.nullOr types.str;
              default = null;
              description = "Google Spreadsheet ID for sync";
            };

            googleSheetName = mkOption {
              type = types.str;
              default = "Expenses";
              description = "Base name for the Google Sheet (year prefixed automatically)";
            };

            googleServiceAccountFile = mkOption {
              type = types.nullOr types.path;
              default = null;
              description = "Path to Google service account JSON file";
            };

            syncInterval = mkOption {
              type = types.str;
              default = "30s";
              description = "Interval for sync processor";
            };

            recurringInterval = mkOption {
              type = types.str;
              default = "1h";
              description = "Interval for recurring expense processor";
            };

            environmentFile = mkOption {
              type = types.nullOr types.path;
              default = null;
              description = "Path to environment file with secrets";
            };
          };

          config = mkIf cfg.enable {
            systemd.services.spese = {
              description = "Spese expense tracker";
              wantedBy = [ "multi-user.target" ];
              after = [ "network.target" ];

              serviceConfig = {
                Type = "simple";
                ExecStart = "${cfg.package}/bin/spese";
                Restart = "always";
                RestartSec = 5;

                # Hardening
                DynamicUser = true;
                StateDirectory = "spese";
                StateDirectoryMode = "0750";
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
                MemoryDenyWriteExecute = true;
                LockPersonality = true;
              } // optionalAttrs (cfg.environmentFile != null) {
                EnvironmentFile = cfg.environmentFile;
              };

              environment = {
                PORT = toString cfg.port;
                SQLITE_DB_PATH = "${cfg.dataDir}/spese.db";
                SYNC_INTERVAL = cfg.syncInterval;
                RECURRING_PROCESSOR_INTERVAL = cfg.recurringInterval;
                DATA_BACKEND = "sqlite";
                GOOGLE_SHEET_NAME = cfg.googleSheetName;
              } // optionalAttrs (cfg.googleSpreadsheetId != null) {
                GOOGLE_SPREADSHEET_ID = cfg.googleSpreadsheetId;
              } // optionalAttrs (cfg.googleServiceAccountFile != null) {
                GOOGLE_SERVICE_ACCOUNT_FILE = toString cfg.googleServiceAccountFile;
              };
            };
          };
        };
    };
}
