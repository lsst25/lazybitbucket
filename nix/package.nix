{
  lib,
  buildGoApplication,
  version ? (
    if builtins.pathExists ../.git then
      builtins.substring 0 7 (lib.commitIdFromGitRepo ../.git)
    else
      "dev"
  ),
}:
buildGoApplication {
  inherit version;

  pname = "lazybitbucket";
  src = ./..;
  modules = ../gomod2nix.toml;
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];
  subPackages = [ "cmd/lazybitbucket" ];
  meta = with lib; {
    description = "Terminal UI for Bitbucket Cloud pull requests";
    homepage = "https://github.com/lsst25/lazybitbucket";
    license = licenses.mit;
    mainProgram = "lazybitbucket";
  };
}
