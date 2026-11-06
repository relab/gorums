# A minimal SSH host for the sweep smoke test: sweep uploads and runs its own
# binaries, so the image needs only sshd and the tools sweep runs on its hosts
# (ss, ip, pgrep, pkill, free, setsid, netstat, ping, rsync, and bash, which
# the base image has).
FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        iproute2 iputils-ping net-tools openssh-server procps rsync util-linux \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /run/sshd \
    && ssh-keygen -A \
    && useradd --create-home --shell /bin/bash smoke \
    && install -d -m 700 -o smoke -g smoke /home/smoke/.ssh \
    && printf '%s\n' \
        'PasswordAuthentication no' \
        'KbdInteractiveAuthentication no' \
        'PermitRootLogin no' \
        'AllowAgentForwarding yes' \
        'AllowTcpForwarding yes' \
        'MaxSessions 64' \
        'MaxStartups 64' \
        > /etc/ssh/sshd_config.d/smoke.conf

# The throwaway public key of one smoke run. It comes last, so that a new key
# rebuilds only this layer.
ARG SMOKE_PUBKEY
RUN test -n "$SMOKE_PUBKEY" \
    && printf '%s\n' "$SMOKE_PUBKEY" > /home/smoke/.ssh/authorized_keys \
    && chown smoke:smoke /home/smoke/.ssh/authorized_keys \
    && chmod 600 /home/smoke/.ssh/authorized_keys

CMD ["/usr/sbin/sshd", "-D", "-e"]
