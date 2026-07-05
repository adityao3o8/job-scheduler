FROM node:20-alpine
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --silent
COPY . .
ARG API_URL=http://api:8080
ENV NEXT_PUBLIC_API_URL=$API_URL
RUN npm run build
EXPOSE 3000
CMD ["npx", "next", "start"]
