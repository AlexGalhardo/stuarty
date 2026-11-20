You need a Google account, and a Google Cloud project that you create. Stuarty uses the redirect address `<your Stuarty address>/social/callback`, for example `http://127.0.0.1:7878/social/callback` on a local server. The Connections page of Stuarty shows the exact value to paste.

1. Open the Google Cloud console and create a project.
2. Enable **YouTube Data API v3** for it.
3. Configure the consent screen as **External**, and add your own Google address as a **test user**.
4. Create an **OAuth client** of the type **Web application**, and paste the redirect address above as an **authorised redirect URI**.
5. In Stuarty, open **Connections**, choose **Connect**, pick YouTube, and paste the client id and the client secret. The secret goes into the vault and is never shown again.
6. Choose the sign-in. Google asks you to accept the permissions, and sends you back to Stuarty.

Two things to know. While the consent screen is in the status **Testing**, Google may expire the refresh token after a few days; the connection then shows that it needs a new sign-in, and you sign in again. And Stuarty always sends a visibility with the video (`private`, `unlisted` or `public`, your choice per post), so a video is never public by accident.

A video upload counts against the daily quota of your API project; the limits below are the ones Stuarty checks before it sends anything.
