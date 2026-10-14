{
  lib,
  buildGo126Module,
  version ? "unstable",
}:

buildGo126Module {
  pname = "nvueschema";
  inherit version;

  src = lib.fileset.toSource {
    root = ../.;
    fileset = lib.fileset.unions [
      ../go.mod
      ../go.sum
      ../LICENSE
      ../testdata
      (lib.fileset.fileFilter (file: file.hasExt "go" || file.name == "nvue-patterns.txt") ../.)
    ];
  };

  vendorHash = "sha256-LoGCmJwMPM6o6T5q2vP0DpkQ7UxZACONg2seNRJDdKs=";
  subPackages = [ "cmd/nvueschema" ];
  env.CGO_ENABLED = "0";
  __darwinAllowLocalNetworking = true;
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];

  # Keep all package unit tests, including packages not installed as commands.
  checkPhase = ''
    runHook preCheck
    # The generated-code type checks use go/importer, which needs GOROOT.
    export GOROOT="$(go env GOROOT)"
    go test ./...
    runHook postCheck
  '';

  doInstallCheck = true;
  installCheckPhase = ''
    runHook preInstallCheck
    "$out/bin/nvueschema" --version | grep -F "${version}"
    "$out/bin/nvueschema" --help > /dev/null
    runHook postInstallCheck
  '';

  meta = {
    description = "Generate NVUE schemas and validate Cumulus Linux configurations";
    homepage = "https://github.com/nemith/nvueschema";
    license = lib.licenses.mit;
    mainProgram = "nvueschema";
    platforms = lib.platforms.linux ++ lib.platforms.darwin;
  };
}
