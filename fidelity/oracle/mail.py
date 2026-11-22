#!/usr/bin/env python3
"""Write mail/testdata/headers.json, the folded headers of policy.default."""

from __future__ import annotations

import json
import sys
from email import policy
from email.message import EmailMessage
from pathlib import Path

if sys.version_info < (3, 14):
    sys.exit("Run this with Python 3.14 or later: the goldens follow the email package of 3.14.")

OUT = Path(__file__).resolve().parent.parent.parent / "mail" / "testdata" / "headers.json"

LONG_WORDS = " ".join(["word"] * 30)
LONG_ATOM = "x" * 90
UMLAUTS = "Ünïcödé "

UNSTRUCTURED = [
    "",
    "Subject",
    "  leading and trailing  ",
    "tab\there",
    "a" * 70,
    "a" * 69,
    "a" * 71,
    "a" * 77,
    LONG_WORDS,
    LONG_ATOM,
    "short " + LONG_ATOM,
    "Gżegżółka",
    "Ünïcödé " * 12,
    UMLAUTS + LONG_WORDS,
    LONG_WORDS + " " + UMLAUTS,
    "日本語の件名です" * 6,
    "emoji 🎉 party 🎉🎉 time",
    "mixed ascii and ü and more ascii text that keeps going on and on and on",
    "=?utf-8?q?hi?= there",
    "=?utf-8?q?S=C3=BCrname?= =?utf-8?q?_=C3=84bc?=",
    "literal =? and ?= marks",
    "price=?3 and ?=4",
    "Re: [list] (comment) <angle> @ , ; : \\ \" quoted",
    "a\x01b control",
    "trailing space ",
    "x" * 30 + " " + "ü" * 30 + " " + "y" * 30,
    "Subject: " + "Ä" * 40,
    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa ü",
    "a" * 76 + " ü",
    "a" * 77 + " üü",
    "a" * 78 + " ü",
    "Long subject with a ünïcödé word near the end of a line that wraps around",
]

ADDRESSES = [
    "",
    "to@example.com",
    "Firstname Sürname <to@example.com>",
    '"Firstname Sürname" <to@example.com>',
    '"Lastname, Firstname" <to@example.com>',
    "Sürname, Firstname <to@example.com>",
    "to@éxample.com",
    "Firstname Sürname <to@éxample.com>",
    "tó@example.com",
    "Tó Example <tó@example.com>",
    "to@example.com, other@example.com",
    ", ".join(f"recipient{i}@example.com" for i in range(8)),
    ", ".join(f"Recipient Number {i} <recipient{i}@example.com>" for i in range(5)),
    '"Lastname, Firstname" <to@example.com>, Firstname Sürname <x@example.com>, "Sürname, Äbc" <y@example.com>',
    "undisclosed-recipients:;",
    "Group: a@example.com, b@example.com;",
    "Group: Sürname <a@example.com>;, c@example.com",
    "to@example.com (comment)",
    "(lead) to@example.com",
    "<to@example.com>",
    "<>",
    "to",
    "to@",
    "@example.com",
    "to@example.com>",
    "<to@example.com",
    "to@@example.com",
    "to@example..com",
    "to.@example.com",
    ".to@example.com",
    "to..x@example.com",
    "Name <to@example.com> trailing",
    "to@example.com; other@example.com",
    "to@[127.0.0.1]",
    "to@[IPv6:::1]",
    '"quoted local"@example.com',
    '"weird\\"quote"@example.com',
    "a b@example.com",
    "foo@example.com\x00",
    "to\x00@example.com",
    "Name <to@example.com>, <other@example.com>",
    "<@route,@other:to@example.com>",
    "=?utf-8?q?S=C3=BCrname?= <to@example.com>",
    '"=?utf-8?q?S=C3=BCrname?=" <to@example.com>',
    "A Very Long Display Name That Goes On And On And On For Quite A While <someone@example.com>",
    "Ä Very Löng Displäy Nämé Thät Göes Ön Änd Ön Änd Ön För Qüite Ä While <someone@example.com>",
    "Name With Dot. <to@example.com>",
    "x" * 80 + "@example.com",
    "to@" + "sub." * 20 + "example.com",
    "<to@example.com> (comment) ,",
    ",to@example.com",
    "to@example.com,,other@example.com",
    '"A Very Long Display Name With A \\"Quote\\" Inside That Wraps Over The Line" <someone@example.com>',
    '"Back\\\\slash Name That Is Long Enough To Need Folding Across Lines Here" <someone@example.com>',
    '"Very long display name, with comma and ünïcödé that must fold" <x@example.com>',
    "=?utf-8?q?S=C3=BCrname?= =?utf-8?q?_=C3=84bc?= <to@example.com>",
    "=?utf-8?q?S=C3=BCrname?= Plain <to@example.com>",
    "=?utf-8?q?A?=  =?utf-8?q?B?= <to@example.com>",
    "Plain =?utf-8?q?S=C3=BCrname?= <to@example.com>",
    '"" <to@example.com>',
    ".foo <a@example.com>",
    "(c) .foo <a@example.com>",
    "to@[",
    "to@[1.2",
    "<@route:to@example.com>",
    "<@route,,@other:to@example.com>",
    "<@:to@example.com>",
    "<,@route:to@example.com>",
    "to@example.com junk, other@example.com",
    "Name <to@example.com> junk <x@example.com>",
    "a" * 70 + " Ü <to@example.com>",
    "a" * 76 + " Ü <to@example.com>",
]

MESSAGE_IDS = [
    "<12345.678.9@example.com>",
    "foo",
    "<foo>",
    "<foo@bar",
    "<a.b@[127.0.0.1]>",
    "<" + "x" * 90 + "@example.com>",
    "  <spaced@example.com>  ",
    "<x@example.com> trailing",
]

REFERENCES = [
    "",
    "<a@example.com>",
    "<a@example.com> <b@example.com>",
    "<a@example.com>, <b@example.com>",
    "<a@example.com> junk",
    "junk",
    "<a@example.com> (comment) <b@example.com>",
    "<a@[127.0.0.1]>",
    "<ü@example.com>",
    " ".join(f"<id{i}.{'x' * 20}@example.com>" for i in range(5)),
]

DATES = [
    "Fri, 09 Nov 2001 01:08:47 -0000",
    "Fri, 09 Nov 2001 01:08:47 +0100",
    "Tue, 31 Dec 2024 23:59:59 GMT",
    "9 Nov 2001 01:08:47 -0500",
    "garbage",
]

HEADERS = (
    [("Subject", v) for v in UNSTRUCTURED]
    + [("X-Custom-Header", v) for v in UNSTRUCTURED[:12]]
    + [("A-Name-That-Is-Rather-Long-For-A-Header", "with a value that pushes the line over")]
    + [("To", v) for v in ADDRESSES]
    + [("From", v) for v in ADDRESSES[:10]]
    + [("Reply-To", v) for v in ADDRESSES[:4]]
    + [("Message-ID", v) for v in MESSAGE_IDS]
    + [("Date", v) for v in DATES]
    + [("Resent-Date", "garbage")]
    + [("References", v) for v in REFERENCES]
    + [("In-Reply-To", v) for v in REFERENCES[:5]]
)

HEADER_NAMES = [
    "Subject",
    "X-Custom-Header",
    "~!#$%&'*+-.^_`|",
    "",
    " ",
    "Bad Name",
    "Bad:Name",
    "Name:",
    "Name\n",
    "Name\n\n",
    "Name\r",
    "\nName",
    "Näme",
    "Na\x7fme",
]

UNIQUE_NAMES = [
    "Subject",
    "Date",
    "Resent-Date",
    "To",
    "Cc",
    "Bcc",
    "From",
    "Reply-To",
    "Sender",
    "Resent-To",
    "Message-ID",
    "In-Reply-To",
    "References",
    "MIME-Version",
    "Content-Type",
    "Content-Transfer-Encoding",
    "X-Custom-Header",
    "Received",
]


def mailbox_info(mb) -> dict:
    """Return the accessors of a parsed mailbox."""
    return {
        "type": mb.token_type,
        "display_name": mb.display_name,
        "local_part": mb.local_part,
        "domain": mb.domain,
        "addr_spec": mb.addr_spec,
    }


def case(name: str, value: str) -> dict:
    """Return the folded forms of one header and its parsed addresses."""
    try:
        header = policy.default.header_factory(name, value)
    except Exception as e:  # noqa: BLE001
        return {"name": name, "value": value, "error": type(e).__name__}
    out = {
        "name": name,
        "value": value,
        "str": str(header),
        "default": header.fold(policy=policy.default),
        "smtp": header.fold(policy=policy.SMTP),
    }
    if name in ("To", "From", "Reply-To"):
        from email._header_value_parser import get_address_list

        tree = get_address_list(value)[0] if value else None
        out["mailboxes"] = [mailbox_info(mb) for mb in tree.all_mailboxes] if tree else []
        out["defects"] = [str(d) for d in tree.all_defects] if tree else []
        out["tree_str"] = str(tree) if tree else ""
    return out


def name_case(name: str) -> dict:
    """Return the error of setting a header with this name, or ""."""
    try:
        EmailMessage()[name] = "value"
    except ValueError as e:
        return {"name": name, "error": str(e)}
    return {"name": name, "error": ""}


def is_unique(name: str) -> bool:
    """Report whether a message refuses a second header with this name."""
    msg = EmailMessage()
    msg[name] = "1.0" if name == "MIME-Version" else "<a@example.com>"
    try:
        msg[name] = "1.0" if name == "MIME-Version" else "<a@example.com>"
    except ValueError:
        return True
    return False


def main() -> None:
    """Write the golden file."""
    golden = {
        "headers": [case(name, value) for name, value in HEADERS],
        "names": [name_case(name) for name in HEADER_NAMES],
        "unique": {name: is_unique(name) for name in UNIQUE_NAMES},
    }
    OUT.write_text(json.dumps(golden, ensure_ascii=False, indent=1) + "\n")


if __name__ == "__main__":
    main()
