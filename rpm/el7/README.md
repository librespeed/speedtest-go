# librespeedgo-rpm

Librespeedtest Go version package (tested for el7)
upstream: https://github.com/librespeed/speedtest-go

custom rpmmacro vars:
* hk_version - define version
* hk_build - define build
* godir - change default GOPATH

example:
```
rpmbuild -D 'hk_build 3' -D 'hk_version 1.1.3' -D 'godir %{_builddir}/%{name}/.go' -bb SPECS/librespeedgo.spec
```
