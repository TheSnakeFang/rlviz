"use strict";

const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const test = require("node:test");
const { download, expectedChecksum, install, releaseTarget, releaseURLs, MAX_CHECKSUM_BYTES } = require("./install.cjs");

test("maps supported release targets", () => {
  assert.deepEqual(releaseTarget("darwin", "arm64"), { os: "darwin", arch: "arm64" });
  assert.deepEqual(releaseTarget("linux", "x64"), { os: "linux", arch: "x86_64" });
  assert.throws(() => releaseTarget("win32", "x64"), /unsupported platform/);
});

test("builds the GoReleaser archive URL", () => {
  assert.deepEqual(releaseURLs("v0.1.0", { os: "linux", arch: "arm64" }, "https://example.test/releases/"), {
    archive: "rlviz_0.1.0_linux_arm64.tar.gz",
    archiveURL: "https://example.test/releases/v0.1.0/rlviz_0.1.0_linux_arm64.tar.gz",
    checksumsURL: "https://example.test/releases/v0.1.0/checksums.txt",
  });
  assert.throws(() => releaseURLs("../../bad", { os: "linux", arch: "arm64" }), /invalid rlviz package version/);
});

function fixtureArchive(t, entry = "file") {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "rlviz-npm-test-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const payload = path.join(root, "payload");
  const packageDirectory = path.join(root, "package");
  fs.mkdirSync(payload);
  fs.mkdirSync(path.join(packageDirectory, "bin"), { recursive: true });
  if (entry === "symlink") fs.symlinkSync("/bin/sh", path.join(payload, "rlviz"));
  else fs.writeFileSync(path.join(payload, "rlviz"), "#!/bin/sh\necho native-fixture\n", { mode: 0o755 });
  fs.writeFileSync(path.join(payload, "ignored"), "must not be extracted");
  const archive = "rlviz_1.2.3_linux_x86_64.tar.gz";
  const archivePath = path.join(root, archive);
  const packed = spawnSync("tar", ["-czf", archivePath, "-C", payload, "rlviz", "ignored"], { encoding: "utf8" });
  assert.equal(packed.status, 0, packed.stderr);
  const archiveData = fs.readFileSync(archivePath);
  const digest = crypto.createHash("sha256").update(archiveData).digest("hex");
  const downloads = new Map([[archive, archiveData], ["checksums.txt", Buffer.from(`${digest}  ${archive}\n`)]]);
  const download = async (url) => downloads.get(path.basename(new URL(url).pathname));
  return { archive, archiveData, download, downloads, packageDirectory, root };
}

test("downloads, verifies, extracts only rlviz, and atomically replaces the shim", async (t) => {
  const fixture = fixtureArchive(t);
  const destination = path.join(fixture.packageDirectory, "bin", "rlviz");
  fs.writeFileSync(destination, "old shim");
  await install({
    packageDirectory: fixture.packageDirectory,
    packageVersion: "1.2.3",
    platform: "linux",
    architecture: "x64",
    releaseBaseURL: "https://example.test/releases",
    download: fixture.download,
  });
  assert.match(fs.readFileSync(destination, "utf8"), /native-fixture/);
  assert.equal(fs.statSync(destination).mode & 0o777, 0o755);
  assert.equal(fs.existsSync(path.join(fixture.packageDirectory, "ignored")), false);
});

test("rejects checksum mismatch without replacing the shim", async (t) => {
  const fixture = fixtureArchive(t);
  fixture.downloads.set("checksums.txt", Buffer.from(`${"0".repeat(64)}  ${fixture.archive}\n`));
  const destination = path.join(fixture.packageDirectory, "bin", "rlviz");
  fs.writeFileSync(destination, "safe shim");
  await assert.rejects(install({ packageDirectory: fixture.packageDirectory, packageVersion: "1.2.3", platform: "linux", architecture: "x64", releaseBaseURL: "https://example.test/releases", download: fixture.download }), /checksum verification failed/);
  assert.equal(fs.readFileSync(destination, "utf8"), "safe shim");
});

test("rejects a symlink in place of the native binary", async (t) => {
  const fixture = fixtureArchive(t, "symlink");
  await assert.rejects(install({ packageDirectory: fixture.packageDirectory, packageVersion: "1.2.3", platform: "linux", architecture: "x64", releaseBaseURL: "https://example.test/releases", download: fixture.download }), /safe rlviz binary/);
});

test("selects an exact checksum entry", () => {
  const digest = "a".repeat(64);
  assert.equal(expectedChecksum(`${"b".repeat(64)}  other.tar.gz\n${digest}  rlviz.tar.gz\n`, "rlviz.tar.gz"), digest);
  assert.equal(expectedChecksum(`${digest}${" ".repeat(10_000)}*rlviz.tar.gz\n`, "rlviz.tar.gz"), digest);
  assert.throws(() => expectedChecksum(`${digest}  other.tar.gz\n`, "rlviz.tar.gz"), /not found/);
});

test("refuses insecure downloads and oversized injected responses", async (t) => {
  await assert.rejects(download("http://example.test/archive", 10), /non-HTTPS/);
  const fixture = fixtureArchive(t);
  const oversized = async (url) => path.basename(new URL(url).pathname) === "checksums.txt"
    ? Buffer.alloc(MAX_CHECKSUM_BYTES + 1)
    : fixture.archiveData;
  await assert.rejects(install({ packageDirectory: fixture.packageDirectory, packageVersion: "1.2.3", platform: "linux", architecture: "x64", releaseBaseURL: "https://example.test/releases", download: oversized }), /oversized checksum file/);
});
