FROM alpine:3.19
RUN apk add --no-cache iproute2
# tc runs in an ephemeral container sharing the target's network namespace.
CMD ["tc", "-V"]
