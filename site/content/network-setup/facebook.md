Facebook publishes a **Reel** to a Page that you manage, through a Meta app that you create. It can be the same app as for Instagram.

1. Open the Meta for Developers console and create an app of the type **Business** (or use the one you made for Instagram).
2. Add **Facebook Login for Business** (or Facebook Login), and paste `<your Stuarty address>/social/callback` as a valid OAuth redirect address. The Connections page of Stuarty shows the exact value.
3. Keep the app in **development mode**, with yourself as an administrator.
4. In Stuarty, open **Connections**, choose **Connect**, pick Facebook, and paste the app id and the app secret. The secret goes into the vault.
5. Sign in, accept every permission, and **choose the Page** from the list Stuarty shows.

A Page video that is not a Reel (longer than 90 seconds, or not 9:16) is not offered: Stuarty refuses the post and names the limit, so nothing is sent. The Facebook pages state no largest file and no longest description, so Stuarty applies the stricter limits of Instagram to the text and a ceiling of 1 GiB to the file; the table below says which numbers are Facebook's and which are Stuarty's own.
