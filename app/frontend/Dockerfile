# Build stage
FROM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

# Runtime stage
FROM node:24-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=3000
COPY --from=build /app/dist ./dist
COPY --from=build /app/static ./static
COPY package.json package-lock.json ./
RUN npm ci --omit=dev
USER node
EXPOSE 3000
CMD ["node", "dist/server.js"]
