# <Product> UI coverage matrix

Status: `covered` = a passing test exists (the test column holds part of its title); `fixme` = a product failure marked `test.fixme` (the test column holds part of its title, and the cause is in the test); `gap` = no UI test (the test column holds the reason).

| id | screen | action | status | test |
| --- | --- | --- | --- | --- |
| 1 | LoginScreen.tsx | signs in through the form | covered | a user signs in and signs out |
| 2 | Dashboard.tsx | a failed load shows the error state | covered | the dashboard shows the error state |
| 3 | Plans.tsx | plan editor | gap | not routed; reachable through the API only |
