# Test image for the Helm chart's install tests: the goned that task
# build-prod made (build with bin/ as the context), on a public distroless base. The release image's dhi.io
# base needs a registry login that forked pull requests don't have.
# distroless/static-debian13:nonroot
FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
COPY --chmod=0555 goned /usr/local/bin/goned
USER 65532:65532
EXPOSE 8080 9090
ENTRYPOINT ["/usr/local/bin/goned"]
