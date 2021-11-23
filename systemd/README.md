# Example systemd unit files

To use these, first review the speedtest.* unit files, and then:

    cp ../speedtest /usr/local/bin/
    mkdir -p /usr/local/share/speedtest /usr/local/etc
    cp -aR ../web/assets /usr/local/share/speedtest/assets
    cp speedtest-settings.toml /usr/local/etc
    cp speedtest.* /etc/systemd/system/
    systemctl daemon-reload

If you wish to use the bolt database type:

    # Create static system user and group
    adduser --system --group --no-create-home --disabled-password speedtest
    mkdir -p /usr/local/var/speedtest
    touch /usr/local/var/speedtest/speedtest.db
    chown speedtest. /usr/local/var/speedtest/speedtest.db

To start (and enable at boot-up):

    systemctl enable --now speedtest.socket

speedtest-go should now be listening for http request on port 80 on the local
machine.

You will need to customise the html files e.g. edit
`/usr/local/share/speedtest/assets/index.html` to suit your site.
