"""Promote a tested Docker Hub index to its numbered, commit, and latest tags."""

import json
import re
import subprocess
import sys
from urllib.parse import urlencode, urljoin
from urllib.request import Request, urlopen


REPOSITORY = "tiger2896/cat_two"
IMAGE = f"docker.io/{REPOSITORY}"
TAG_LIST_URL = f"https://registry-1.docker.io/v2/{REPOSITORY}/tags/list"
RUN_LABEL = "io.github.tiger-zero.cat-two.release-run"
REVISION_LABEL = "org.opencontainers.image.revision"


def registry_tags():
    query = urlencode({"service": "registry.docker.io", "scope": f"repository:{REPOSITORY}:pull"})
    with urlopen(f"https://auth.docker.io/token?{query}", timeout=30) as response:
        token = json.load(response)["token"]

    url = f"{TAG_LIST_URL}?n=100"
    while url:
        request = Request(url, headers={"Authorization": f"Bearer {token}"})
        with urlopen(request, timeout=30) as response:
            page = json.load(response)
            link = response.headers.get("Link")
        if page["name"] != REPOSITORY or not isinstance(page["tags"], list):
            raise ValueError("Unexpected Docker Hub tag list")
        yield from page["tags"]

        if not link:
            break
        next_page = re.fullmatch(r'<([^>]+)>;\s*rel="next"', link)
        if next_page is None:
            raise ValueError("Unexpected Docker Hub pagination link")
        url = urljoin(url, next_page[1])
        if not url.startswith(f"{TAG_LIST_URL}?"):
            raise ValueError("Unexpected Docker Hub pagination URL")


def last_number(tags):
    numbers = (
        int(tag.split(".", maxsplit=1)[0]) * 100 + int(tag[-2:])
        for tag in tags
        if re.fullmatch(r"(0|[1-9][0-9]*)\.[0-9]{2}", tag)
    )
    return max(numbers, default=0)


def version_tag(number):
    return f"{number // 100}.{number % 100:02d}"


def inspect(tag):
    result = subprocess.run(
        ["docker", "buildx", "imagetools", "inspect", f"{IMAGE}:{tag}", "--format", "{{json .}}"],
        check=True,
        capture_output=True,
        text=True,
    )
    return json.loads(result.stdout)


def promote(tag, digest):
    subprocess.run(
        ["docker", "buildx", "imagetools", "create", "--tag", f"{IMAGE}:{tag}", f"{IMAGE}@{digest}"],
        check=True,
    )


def publish(candidate_digest, run_number, revision):
    previous_number = last_number(registry_tags())
    number = previous_number + 1
    digest = candidate_digest
    if previous_number:
        previous = inspect(version_tag(previous_number))
        platform_labels = [image["config"]["Labels"] for image in previous["image"].values()]
        run_labels = {labels.get(RUN_LABEL) for labels in platform_labels}
        if len(run_labels) != 1:
            raise ValueError("Inconsistent release run labels in the last numbered image")
        previous_run = int(next(iter(run_labels)) or 0)
        if previous_run > run_number:
            raise ValueError("A newer workflow run has already published a numbered image")
        if previous_run == run_number:
            if {labels.get(REVISION_LABEL) for labels in platform_labels} != {revision}:
                raise ValueError("The numbered image from this workflow run belongs to a different commit")
            # Retries repair aliases using the index that already owns the number.
            number = previous_number
            digest = previous["manifest"]["digest"]

    tag = version_tag(number)
    if number != previous_number:
        promote(tag, digest)
    promote(f"sha-{revision}", digest)
    promote("latest", digest)
    return tag


if __name__ == "__main__":
    digest, run, revision = sys.argv[1:]
    if not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
        raise ValueError("Expected the digest of the published multi-platform image")
    if not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ValueError("Expected the complete Git commit SHA")
    tag = publish(digest, int(run), revision)
    print(f"Published {IMAGE}:{tag}, {IMAGE}:sha-{revision} and {IMAGE}:latest")
