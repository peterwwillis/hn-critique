ARG PLAYWRIGHT_VERSION
FROM mcr.microsoft.com/playwright:${PLAYWRIGHT_VERSION}

USER root
RUN mkdir -p /work && chown -R pwuser:pwuser /work

USER pwuser
WORKDIR /work
