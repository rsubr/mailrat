//go:build darwin

#import <AppKit/AppKit.h>
#include <stdbool.h>
#include <string.h>

bool richclipSet(const char *html, const char *text) {
	@autoreleasepool {
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		[pb clearContents];
		BOOL ok = [pb setString:[NSString stringWithUTF8String:html] forType:NSPasteboardTypeHTML];
		ok = [pb setString:[NSString stringWithUTF8String:text] forType:NSPasteboardTypeString] && ok;
		return ok;
	}
}

// richclipGet returns a malloc'd UTF-8 copy of the pasteboard's HTML (html) or plain text, or NULL.
char *richclipGet(bool html) {
	@autoreleasepool {
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		NSString *s = [pb stringForType:(html ? NSPasteboardTypeHTML : NSPasteboardTypeString)];
		if (s == nil) {
			return NULL;
		}
		return strdup([s UTF8String]);
	}
}
