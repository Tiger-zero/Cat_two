import unittest
from unittest.mock import call, patch

import cat_two_docker_release as release


CURRENT = "sha256:" + "b" * 64
PREVIOUS = "sha256:" + "a" * 64
REVISION = "c" * 40
SHA_TAG = f"sha-{REVISION}"


def previous_index(run, revision=REVISION):
    labels = {release.RUN_LABEL: str(run), release.REVISION_LABEL: revision}
    return {
        "image": {
            platform: {"config": {"Labels": labels}}
            for platform in ("linux/amd64", "linux/arm64")
        },
        "manifest": {"digest": PREVIOUS},
    }


class PublishTest(unittest.TestCase):
    def test_first_release(self):
        with patch.object(release, "registry_tags", return_value=[]), patch.object(release, "promote") as promote:
            self.assertEqual(release.publish(CURRENT, 12, REVISION), "0.01")
        self.assertEqual(promote.call_args_list, [call("0.01", CURRENT), call(SHA_TAG, CURRENT), call("latest", CURRENT)])

    def test_next_release(self):
        with (
            patch.object(release, "registry_tags", return_value=["0.99", "latest", "0.16"]),
            patch.object(release, "inspect", return_value=previous_index(10)) as inspect,
            patch.object(release, "promote") as promote,
        ):
            self.assertEqual(release.publish(CURRENT, 12, REVISION), "1.00")
        inspect.assert_called_once_with("0.99")
        self.assertEqual(promote.call_args_list, [call("1.00", CURRENT), call(SHA_TAG, CURRENT), call("latest", CURRENT)])

    def test_retry_uses_previously_numbered_index(self):
        with (
            patch.object(release, "registry_tags", return_value=["0.16"]),
            patch.object(release, "inspect", return_value=previous_index(12)),
            patch.object(release, "promote") as promote,
        ):
            self.assertEqual(release.publish(CURRENT, 12, REVISION), "0.16")
        self.assertEqual(promote.call_args_list, [call(SHA_TAG, PREVIOUS), call("latest", PREVIOUS)])

    def test_stale_or_conflicting_run_cannot_promote(self):
        for run, revision in ((13, REVISION), (12, "d" * 40)):
            with (
                self.subTest(run=run, revision=revision),
                patch.object(release, "registry_tags", return_value=["0.16"]),
                patch.object(release, "inspect", return_value=previous_index(run, revision)),
                patch.object(release, "promote") as promote,
            ):
                with self.assertRaises(ValueError):
                    release.publish(CURRENT, 12, REVISION)
                promote.assert_not_called()


if __name__ == "__main__":
    unittest.main()
