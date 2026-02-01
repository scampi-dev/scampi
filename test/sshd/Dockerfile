FROM alpine:3.19

RUN apk add --no-cache openssh-server shadow

# Create test users and groups (set placeholder password to unlock accounts for SSH)
RUN addgroup -g 1000 testgroup && \
  adduser -D -u 1000 -G testgroup testuser && \
  addgroup -g 1001 othergroup && \
  adduser -D -u 1001 -G othergroup otheruser && \
  echo "testuser:testpass" | chpasswd && \
  echo "otheruser:testpass" | chpasswd

# Setup SSH
RUN mkdir -p /var/run/sshd && \
  ssh-keygen -A

# Configure sshd - add LogLevel DEBUG for troubleshooting
RUN echo "PasswordAuthentication no" >> /etc/ssh/sshd_config && \
  echo "PubkeyAuthentication yes" >> /etc/ssh/sshd_config && \
  echo "PermitRootLogin no" >> /etc/ssh/sshd_config && \
  echo "LogLevel DEBUG" >> /etc/ssh/sshd_config

# Create .ssh directory with correct permissions BEFORE copying key
RUN mkdir -p /home/testuser/.ssh && \
  chown testuser:testgroup /home/testuser/.ssh && \
  chmod 700 /home/testuser/.ssh

# Add test key
COPY --chown=testuser:testgroup --chmod=600 testkey.pub /home/testuser/.ssh/authorized_keys

EXPOSE 22
CMD ["/usr/sbin/sshd", "-D", "-e"]

