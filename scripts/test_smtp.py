import smtplib
from email.mime.text import MIMEText

to = "15e2fe5a5ab3166f@mailinator.local"
msg = MIMEText("Hey")
msg["Subject"] = "Test message"
msg["From"] = "a@foo.com"
msg["To"] = to

with smtplib.SMTP("localhost", 2525) as s:
    s.sendmail("a@foo.com", [to], msg.as_string())
