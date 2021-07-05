%global appname		librespeedgo
%global	debug_package	%{nil} 
%global __os_install_post %(echo '%{__os_install_post}' | sed -e 's!/usr/lib[^[:space:]]*/brp-.*[[:space:]].*$!!g')

Name:		%{appname}
Version:	%{hk_version}
Release:	%{hk_build}%{?dist}
Summary:	LibreSpeed go-backend server

Group:		Applications/System
License:	LGPL
URL:		https://github.com/librespeed/speedtest-go
Source0:	%{name}.tar.gz
Source1:	%{name}.mainconfig
Source2:	%{name}.service
Source3:	%{name}.firewalld

AutoReq:		no
AutoProv:		no
BuildArch:		x86_64
BuildRequires:	golang >= 1.13

%description
Very lightweight speed test implemented in Javascript, using XMLHttpRequest and Web Workers.

%prep
curl -sL 'https://github.com/librespeed/speedtest-go/archive/refs/tags/v%{version}.tar.gz' -o %{_sourcedir}/%{name}.tar.gz
if [[ -d %{_builddir}/%{name} ]];then
	chmod 777 -R %{_builddir}/%{name}
	rm -rf %{_builddir}/%{name}
fi
mkdir %{_builddir}/%{name}
tar xf %{_sourcedir}/%{name}.tar.gz -C %{_builddir}/%{name} --strip-components 1
cd %{_builddir}/%{name}
cat << EOF >> %{name}.runtime
d /var/lib/librespeedgo 0750 librespeedgo librespeedgo
f /etc/librespeedgo/settings.toml 0640 root librespeedgo
f /var/lib/librespeedgo/speedtest.db 0640 librespeedgo librespeedgo
EOF
cp -a %{SOURCE1} %{SOURCE2} %{SOURCE3} ./
pushd %{_builddir}/%{name}/assets
sed -i "s/LibreSpeed Example/LibreSpeed/" *.html
popd

%build
pushd %{_builddir}/%{name}
%if 0%{?godir:1}
GOPATH=%{godir} go build -ldflags "-w -s" -trimpath -o %{name} main.go
%else
go build -ldflags "-w -s" -trimpath -o %{name} main.go
%endif
popd

%install
pushd %{_builddir}/%{name}
install -D %{name}                     %{buildroot}%{_bindir}/%{name}
install -Dm644 %{name}.runtime         %{buildroot}%{_sysconfdir}/tmpfiles.d/%{name}.conf
install -Dm640 %{name}.mainconfig      %{buildroot}%{_sysconfdir}/%{name}/settings.toml
install -Dm644 %{name}.service         %{buildroot}%{_prefix}/lib/systemd/system/%{name}.service
install -Dm644 %{name}.firewalld       %{buildroot}%{_prefix}/lib/firewalld/services/%{name}.xml
install -dm750                         %{buildroot}/var/lib/%{name}

install -d                                                 %{buildroot}/%{_datadir}/%{name}
cp -r assets                                               %{buildroot}/%{_datadir}/%{name}
install -m644 database/mysql/telemetry_mysql.sql           %{buildroot}/%{_datadir}/%{name}
install -m644 database/postgresql/telemetry_postgresql.sql %{buildroot}/%{_datadir}/%{name}
popd

%files
%config(noreplace) %{_sysconfdir}/%{name}/settings.toml
%config(noreplace) %{_prefix}/lib/firewalld/services/%{name}.xml
%config %{_sysconfdir}/tmpfiles.d/%{name}.conf
%config %{_prefix}/lib/systemd/system/%{name}.service
%{_bindir}/%{name}
%{_datadir}/%{name}
/var/lib/%{name}

%post
if [ $1 == 1 ];then
  if ! getent passwd %{name} > /dev/null; then
        useradd -r -s /bin/false -m -d /var/lib/%{name} %{name}
  fi
  touch /var/lib/%{name}/speedtest.db
  chown -R %{name}:%{name} /var/lib/%{name}
  systemctl daemon-reload
elif [ $1 == 2 ];then
  chown -R %{name}:%{name} /var/lib/%{name}
  systemctl daemon-reload
  if [ $(systemctl is-active --quiet %{name}.service) ];then
    systemctl restart %{name}.service
  fi
fi

%preun
if [ $1 == 0 ];then
  if [ $(systemctl is-active --quiet %{name}.service) ];then
    systemctl stop %{name}.service
  fi
fi

%changelog
