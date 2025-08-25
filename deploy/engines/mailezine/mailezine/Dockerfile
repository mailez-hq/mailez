# Marker for build-images component discovery only.
#
# The mailezine engine lives in the sibling repository (../mailezine from the
# mailez repo root) with its own Dockerfile and go.mod. build-images detects
# this component by the directory name "mailezine" and builds it from that
# external context; building this file directly is a mistake, so it fails
# loudly instead of producing a broken image.
FROM scratch
RUN echo "build mailezine via backend/cmd/build-images (context: ../mailezine), not this file"
