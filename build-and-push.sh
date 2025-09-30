#!/bin/bash

# Script to build and push s3_exporter Docker image to Artifactory
# Usage: ./build-and-push.sh [version] [artifactory-url]

set -e

VERSION=${1:-$(cat VERSION)}
ARTIFACTORY_URL=${2:-"your-artifactory.com"}
IMAGE_NAME="s3-exporter"
FULL_IMAGE_NAME="${ARTIFACTORY_URL}/${IMAGE_NAME}:${VERSION}"
LATEST_IMAGE_NAME="${ARTIFACTORY_URL}/${IMAGE_NAME}:latest"

echo "Building s3_exporter version ${VERSION}..."

# Build the Docker image
docker build -t "${IMAGE_NAME}:${VERSION}" .
docker tag "${IMAGE_NAME}:${VERSION}" "${FULL_IMAGE_NAME}"
docker tag "${IMAGE_NAME}:${VERSION}" "${LATEST_IMAGE_NAME}"

echo "Built image: ${FULL_IMAGE_NAME}"
echo "Built image: ${LATEST_IMAGE_NAME}"

# Optional: Push to Artifactory (uncomment when ready)
# echo "Pushing to Artifactory..."
# docker push "${FULL_IMAGE_NAME}"
# docker push "${LATEST_IMAGE_NAME}"
# echo "Successfully pushed ${FULL_IMAGE_NAME} and ${LATEST_IMAGE_NAME}"

echo "To push to Artifactory, run:"
echo "  docker push ${FULL_IMAGE_NAME}"
echo "  docker push ${LATEST_IMAGE_NAME}"

echo ""
echo "To deploy to Kubernetes:"
echo "  1. Update k8s/secret.yaml.template with your S3 credentials"
echo "  2. Update k8s/deployment.yaml with the correct image name: ${FULL_IMAGE_NAME}"
echo "  3. Apply: kubectl apply -f k8s/"