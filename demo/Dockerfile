# Demo-only images, built from the repository root:
#   docker build -f demo/Dockerfile --target proxy .
# The app and mock-qui images come from the main Dockerfile. Base images are
# pinned by digest; Dependabot keeps them current.

FROM nginxinc/nginx-unprivileged:1.31.7-alpine@sha256:b5047b9b76845eac124358373c6852377c52e4b05834a47647e955dd91cb8f1e AS proxy
COPY demo/nginx.conf /etc/nginx/conf.d/default.conf

FROM node:26-alpine@sha256:40cd011b55415bc894e45b02fdd5a5dfde056921b7d23a64c74f80497af48797 AS seed
COPY scripts/demo-seed.mjs /demo-seed.mjs
USER node
ENTRYPOINT ["node", "/demo-seed.mjs"]
