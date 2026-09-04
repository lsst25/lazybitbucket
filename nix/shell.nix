{
  mkShell,
  go,
  gomod2nix,
}:
mkShell {
  name = "lazybitbucket-dev";
  packages = [
    go
    gomod2nix
  ];
}
