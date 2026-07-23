# Calibration fixture: keep a Unix-domain listener alive after the command
# returns. Perl ships with the supported Ubuntu base image, avoiding an
# implicit package install or image pull for this container-side probe.
perl -MIO::Socket::UNIX -MSocket=SOCK_STREAM -e '
my $path = "branch-listener.sock";
unlink $path;
my $listener = IO::Socket::UNIX->new(
  Type => SOCK_STREAM,
  Local => $path,
  Listen => 1,
) or die "listen $path: $!\n";
open my $pid_file, ">", "branch-listener-pid.txt" or die "write pid: $!\n";
print {$pid_file} "$$\n";
close $pid_file;
sleep 3;
' >/dev/null 2>&1 &
