# Build stage
FROM node:25-alpine AS builder

WORKDIR /app

COPY services/mddb-chat-widget/package.json services/mddb-chat-widget/package-lock.json* ./

RUN npm install

COPY services/mddb-chat-widget/ .

RUN npm run build

# Runtime stage — serve static files via nginx
FROM nginx:alpine

COPY --from=builder /app/dist/ /usr/share/nginx/html/

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
