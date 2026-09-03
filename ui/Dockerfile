FROM node:26-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.29-alpine
# nginx substitutes BFF_HOST and BFF_PORT into the template at startup, so the
# same image works against any release name.
ENV BFF_HOST=inside-man-bff \
    BFF_PORT=8080
COPY nginx.conf /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 8080
