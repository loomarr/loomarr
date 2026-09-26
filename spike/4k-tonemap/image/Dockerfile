# TEST ONLY — never pushed to any registry. Production image + an OpenCL runtime for tonemap_opencl.
# intel-opencl-icd is not in Debian trixie: VARIANT=intel installs Intel's pinned compute-runtime release debs
# (fetched into debs/ by fetch-debs.sh); VARIANT=rusticl installs Debian's mesa-opencl-icd.
# Mesa Vulkan ICDs are already in the production image.
ARG BASE
FROM ${BASE}
ARG VARIANT=intel
USER root
COPY debs/ /tmp/debs/
RUN set -eux; apt-get -o Acquire::Check-Valid-Until=false update; \
  if [ "$VARIANT" = intel ]; then apt-get install -y --no-install-recommends /tmp/debs/*.deb ocl-icd-libopencl1; \
  else apt-get install -y --no-install-recommends mesa-opencl-icd ocl-icd-libopencl1; fi; \
  rm -rf /tmp/debs /var/lib/apt/lists/*
USER nonroot:nonroot
