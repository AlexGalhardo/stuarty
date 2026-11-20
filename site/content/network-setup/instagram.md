Instagram publishes through a Meta app that you create, and through an Instagram **professional** account (Business or Creator) linked to a Facebook Page.

1. In the Instagram app, convert the account to a professional account (**Settings**, **Account type**), and link it to a Facebook Page (the same page, **Page link**).
2. Open the Meta for Developers console and create an app of the type **Business**.
3. Add **Facebook Login for Business** (or Facebook Login) to the app, and paste `<your Stuarty address>/social/callback` as a valid OAuth redirect address. The Connections page of Stuarty shows the exact value.
4. Keep the app in **development mode**, with yourself as an administrator. For your own accounts this is Standard Access and needs no app review.
5. In Stuarty, open **Connections**, choose **Connect**, pick Instagram, and paste the app id and the app secret. The secret goes into the vault.
6. Sign in, accept every permission, and **choose the account** from the list Stuarty shows.

Stuarty publishes a **Reel** (a video). It sends the file in one resumable upload from your machine and never needs a public address. A cover image is not sent for a Reel, and Stuarty refuses a post that has one, so nothing is silently dropped. If the video is still processing at Instagram when the time to wait ends, publishing the same post again continues without sending the file twice.
