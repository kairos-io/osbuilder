VERSION 0.6

last-commit-packages:
    FROM quay.io/skopeo/stable
    RUN dnf install -y jq
    WORKDIR build
    RUN skopeo list-tags docker://quay.io/kairos/packages | jq -rc '.Tags | map(select( (. | contains("-repository.yaml")) )) | sort_by(. | sub("v";"") | sub("-repository.yaml";"") | sub("-";"") | split(".") | map(tonumber) ) | .[-1]' > REPO_AMD64
    RUN skopeo list-tags docker://quay.io/kairos/packages-arm64 | jq -rc '.Tags | map(select( (. | contains("-repository.yaml")) )) | sort_by(. | sub("v";"") | sub("-repository.yaml";"") | sub("-";"") | split(".") | map(tonumber) ) | .[-1]' > REPO_ARM64
    SAVE ARTIFACT REPO_AMD64 REPO_AMD64
    SAVE ARTIFACT REPO_ARM64 REPO_ARM64

bump-repositories:
    FROM mikefarah/yq
    WORKDIR build
    COPY +last-commit-packages/REPO_AMD64 REPO_AMD64
    COPY +last-commit-packages/REPO_ARM64 REPO_ARM64
    ARG REPO_AMD64=$(cat REPO_AMD64)
    ARG REPO_ARM64=$(cat REPO_ARM64)
    COPY tools-image/luet-amd64.yaml luet-amd64.yaml
    COPY tools-image/luet-arm64.yaml luet-arm64.yaml
    RUN yq eval ".repositories[0] |= . * { \"reference\": \"${REPO_AMD64}\" }" -i luet-amd64.yaml
    RUN yq eval ".repositories[0] |= . * { \"reference\": \"${REPO_ARM64}\" }" -i luet-arm64.yaml
    SAVE ARTIFACT luet-arm64.yaml AS LOCAL tools-image/luet-arm64.yaml
    SAVE ARTIFACT luet-amd64.yaml AS LOCAL tools-image/luet-amd64.yaml

