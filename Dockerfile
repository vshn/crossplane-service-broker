FROM gcr.io/distroless/static:nonroot

COPY crossplane-service-broker /

ENTRYPOINT [ "/crossplane-service-broker" ]
