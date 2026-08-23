$0 == "==================== DEPRECATION NOTICE =====================" {
  suppress = 1
  next
}

suppress && $0 == "=============================================================" {
  suppress = 0
  next
}

!suppress {
  print
}

END {
  if (suppress) {
    exit 1
  }
}
