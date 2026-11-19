export const IMAGE = "ghcr.io/alexgalhardo/stuarty";

/**
 * The one command that starts the newest stable release from the container image (the release workflow moves
 * the latest tag only on a stable release). The flags are those of the Docker page: the port is published on
 * this computer only, the workspace is the /data volume, the container runs in network mode (APP_KEY and a
 * first administrator are required).
 */
export function dockerRunCommand(): string {
	return [
		"docker run --rm -p 127.0.0.1:7878:7878 -v stuarty:/data",
		'-e APP_KEY="$(openssl rand -base64 32)"',
		"-e STUARTY_ADMIN_EMAIL=admin@example.com",
		"-e STUARTY_ADMIN_PASSWORD=choose-a-long-password",
		`${IMAGE}:latest`,
	].join(" \\\n  ");
}
