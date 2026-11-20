**What Stuarty sends to TikTok is a draft, not a post.** The video goes to your TikTok inbox, with no caption, no privacy level and no cover. You tap the notification in the TikTok app, add your caption and publish it there. Stuarty therefore never reports a TikTok publication as published: it ends as **sent as a draft**. TikTok offers no public post through an app that has not been audited, and the audit is meant for a product with a wide public, not for a tool that posts to your own accounts.

1. Create an app in TikTok for Developers.
2. Add **Login Kit** and the **Content Posting API** to it, and ask for the scope `video.upload`.
3. Register `http://127.0.0.1:<port>/social/callback` as the redirect, with the port of your Stuarty server. TikTok allows only a loopback address for it.
4. While the app is not approved, add yourself as a **target user**.
5. Copy the client key and the client secret.
6. In Stuarty, open **Connections**, choose **Connect**, pick TikTok, and paste the client key (the form calls it the client id) and the secret. The secret goes into the vault.
7. Sign in and accept the permissions.

There is no AI disclosure flag in the draft endpoint. If the video has synthetic content, mark it in the TikTok app when you finish the draft; the approval in Stuarty says so.
