import json
import subprocess


def run_grns(env: dict[str, str], *args: str, check: bool = True) -> subprocess.CompletedProcess:
    proc = subprocess.run(
        [env["GRNS_BIN"], *args],
        text=True,
        capture_output=True,
        env=env,
        cwd=env.get("GRNS_REPO_ROOT"),
    )
    if check and proc.returncode != 0:
        raise AssertionError(
            f"command failed ({proc.returncode}): {' '.join(args)}\nstdout={proc.stdout}\nstderr={proc.stderr}"
        )
    return proc


def json_stdout(proc: subprocess.CompletedProcess):
    return json.loads(proc.stdout)
