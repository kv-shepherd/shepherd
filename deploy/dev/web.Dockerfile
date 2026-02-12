FROM node:22-alpine

ARG USER_ID=1000
ARG GROUP_ID=1000

# Create runtime user matching host UID/GID to avoid permission issues on bind mounts.
RUN if [ "${USER_ID}" != "1000" ] || [ "${GROUP_ID}" != "1000" ]; then \
      deluser --remove-home node && \
      addgroup -g "${GROUP_ID}" shepherd && \
      adduser -u "${USER_ID}" -G shepherd -h /home/shepherd -D shepherd; \
    else \
      mkdir -p /home/node && chown -R node:node /home/node; \
    fi

WORKDIR /app
RUN mkdir -p /app && chown -R ${USER_ID}:${GROUP_ID} /app

ENV NEXT_TELEMETRY_DISABLED=1

USER ${USER_ID}:${GROUP_ID}

EXPOSE 3000
CMD ["npm", "run", "dev", "--", "-H", "0.0.0.0"]
