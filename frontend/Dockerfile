FROM node:20-alpine AS build
WORKDIR /app

# Vite bake env var ke bundle saat BUILD time, jadi harus masuk lewat --build-arg,
# bukan cuma runtime env di docker-compose.
ARG VITE_GOOGLE_CLIENT_ID
ENV VITE_GOOGLE_CLIENT_ID=$VITE_GOOGLE_CLIENT_ID

COPY package*.json ./
RUN npm install
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
