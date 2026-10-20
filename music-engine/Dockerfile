FROM node:20-bookworm
WORKDIR /app
COPY package.json ./
RUN npm install --omit=dev --registry=https://registry.npmmirror.com
COPY . .
EXPOSE 23400
CMD ["node", "server.js"]
