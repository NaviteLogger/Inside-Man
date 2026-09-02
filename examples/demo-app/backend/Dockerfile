FROM eclipse-temurin:25-jdk-alpine AS build
WORKDIR /src
COPY Backend.java .
RUN javac -d /out Backend.java

FROM eclipse-temurin:25-jre-alpine
WORKDIR /app
COPY --from=build /out .
EXPOSE 8080
USER nobody
CMD ["java", "Backend"]
