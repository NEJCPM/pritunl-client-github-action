FROM debian:13 AS pritunl-server

RUN apt-get update -qq && \
    apt-get install -qq -y curl gnupg ca-certificates && \
    curl -fsSL https://raw.githubusercontent.com/pritunl/pgp/master/pritunl_repo_pub.asc | \
      gpg --dearmor -o /etc/apt/trusted.gpg.d/pritunl.gpg && \
    echo "deb https://repo.pritunl.com/stable/apt trixie main" > /etc/apt/sources.list.d/pritunl.list && \
    apt-get update -qq && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y pritunl && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 443 1194/udp

CMD ["/entrypoint.sh"]
