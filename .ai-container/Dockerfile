FROM ubuntu:24.04
ENV DEBIAN_FRONTEND=noninteractive

# Sandbox user: created at container startup by the entrypoint using
# the SANDBOX_UID/SANDBOX_GID env vars that runme.sh passes automatically
# (defaults to the host user's id -u / id -g).
# No user is baked into the image so that the same image works for every
# team member without rebuilding.

# Base utilities and capture tools
RUN apt update && apt install -y \
  ca-certificates curl gnupg lsb-release \
  git vim grep mc \
  wget iputils-ping \
  iptables ipset dnsutils \
  openssh-client \
  libcap2-bin \
  openjdk-21-jdk \
  unzip \
  tcpdump \
  tshark && \
  rm -rf /var/lib/apt/lists/*

# Node.js + npm (LTS)
RUN curl -fsSL https://deb.nodesource.com/setup_24.x | bash - && \
  apt update && apt install -y nodejs && \
  rm -rf /var/lib/apt/lists/*

# kubectl
RUN curl -fsSL https://dl.k8s.io/release/stable.txt | \
  xargs -I{} curl -LO https://dl.k8s.io/release/{}/bin/linux/amd64/kubectl && \
  install kubectl /usr/local/bin/kubectl && rm kubectl

# AWS CLI v2
RUN curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o awscliv2.zip && \
  unzip awscliv2.zip && ./aws/install && rm -rf aws awscliv2.zip

# Azure CLI
RUN curl -sL https://aka.ms/InstallAzureCLIDeb | bash

# GitHub CLI
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | \
  dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg && \
  chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg && \
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] \
  https://cli.github.com/packages stable main" \
  > /etc/apt/sources.list.d/github-cli.list && \
  apt update && apt install -y gh && \
  rm -rf /var/lib/apt/lists/*

# GitHub Copilot CLI + Angular CLI
RUN npm install -g @github/copilot @angular/cli

# Install dtctl and dtmgd (latest releases)
RUN set -eux; \
    ARCH=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/'); \
    OS=$(uname -s | tr '[:upper:]' '[:lower:]'); \
    TAG=$(curl -fsSL https://api.github.com/repos/dynatrace-oss/dtctl/releases/latest | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'); \
    curl -fsSL "https://github.com/dynatrace-oss/dtctl/releases/download/${TAG}/dtctl_${TAG#v}_${OS}_${ARCH}.tar.gz" \
      | tar xz -C /usr/local/bin dtctl; \
    TAG=$(curl -fsSL https://api.github.com/repos/dynatrace-oss/dtmgd/releases/latest | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'); \
    curl -fsSL "https://github.com/dynatrace-oss/dtmgd/releases/download/${TAG}/dtmgd_${TAG#v}_${OS}_${ARCH}.tar.gz" \
      | tar xz -C /usr/local/bin dtmgd; \
    chmod +x /usr/local/bin/dtctl /usr/local/bin/dtmgd

# Kiro CLI (optional — only installed when INSTALL_KIRO=1 is passed at build time)
ARG INSTALL_KIRO=0
RUN if [ "$INSTALL_KIRO" = "1" ]; then \
  curl -fsSL https://cli.kiro.dev/install | bash && \
  cp /root/.local/bin/kiro-cli /usr/local/bin/kiro-cli && \
  cp /root/.local/bin/kiro-cli-chat /usr/local/bin/kiro-cli-chat && \
  cp /root/.local/bin/kiro-cli-term /usr/local/bin/kiro-cli-term; \
  fi

COPY refresh-ipset-allowlist.sh /usr/local/bin/
COPY capture-agent-destinations.sh /usr/local/bin/
COPY capture-blocked-traffic.sh /usr/local/bin/
COPY allowlist-domains.txt /tmp/
COPY allowlist-cidrs.txt /tmp/
COPY allowlist-proxy-domains.txt /tmp/
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /usr/local/bin/refresh-ipset-allowlist.sh \
  /usr/local/bin/capture-agent-destinations.sh \
  /usr/local/bin/capture-blocked-traffic.sh \
  /entrypoint.sh

# The sandbox user is NOT created here.
# The entrypoint creates it at startup using SANDBOX_UID/SANDBOX_GID env vars.

ENTRYPOINT ["/entrypoint.sh"]
WORKDIR /workspace
