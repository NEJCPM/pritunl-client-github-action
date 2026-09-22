FROM --platform=linux/amd64 debian:13 AS pritunl-server

RUN export DEBIAN_FRONTEND=noninteractive && \
    apt-get update -qq && \
    apt-get install -qq -y curl gnupg ca-certificates && \
    curl -fsSL https://raw.githubusercontent.com/pritunl/pgp/master/pritunl_repo_pub.asc | \
      gpg --dearmor -o /usr/share/keyrings/pritunl.gpg && \
    gpg --show-keys --with-colons /usr/share/keyrings/pritunl.gpg | grep -q 7568D9BB55FF9E5287D586017AE645C0CF8E292A && \
    echo "deb [ signed-by=/usr/share/keyrings/pritunl.gpg ] https://repo.pritunl.com/stable/apt trixie main" > /etc/apt/sources.list.d/pritunl.list && \
    apt-get update -qq && \
    apt-get install -y --no-install-recommends pritunl procps iproute2 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 443 1194/udp

CMD ["/entrypoint.sh"]
